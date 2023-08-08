/*
Copyright 2023 Chainguard, Inc.
SPDX-License-Identifier: Apache-2.0
*/

package secret

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

// Exported vars for testing.
var (
	Issuer   = "issuer.enforce.dev"
	Buffer   = 10 * time.Minute
	Registry = "cgr.dev"
)

const (
	ttl           = time.Hour
	annotationKey = "pull-secret-updater.chainguard.dev/identity"
)

type Reconciler struct {
	client typedcorev1.CoreV1Interface

	enqueueAfter func(obj interface{}, after time.Duration)
}

type dockerConfig struct {
	Auths map[string]dockerConfigAuth `json:"auths"`
}
type dockerConfigAuth struct {
	Auth []byte `json:"auth"` // user:pass
}

// ReconcileKind implements Interface.ReconcileKind.
func (r *Reconciler) ReconcileKind(ctx context.Context, s *corev1.Secret) reconciler.Event {
	logger := logging.FromContext(ctx).
		With("namespace", s.Namespace, "name", s.Name)

	if s.Annotations == nil || s.Annotations[annotationKey] == "" {
		logger.Debugw("Skipping", "reason", "missing identity label")
		return nil
	}

	if updateIn := checkToken(ctx, s); updateIn > 0 {
		logger.Infow("Enqueueing future refresh",
			"reason", "token valid and not expired",
			"updateIn", updateIn)
		r.enqueueAfter(s, updateIn)
		return nil
	}

	logger.Infof("Token needs update")

	// Get a new token.
	token, err := newToken(ctx, s.Annotations[annotationKey])
	if err != nil {
		logger.Errorf("Failed to get new token: %v", err)
		return err
	}

	cfg := dockerConfig{
		Auths: map[string]dockerConfigAuth{
			Registry: {Auth: []byte("_token:" + token)},
		},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		logger.Errorf("Failed to marshal .dockerconfigjson: %w", err)
		return err
	}
	s.Data[".dockerconfigjson"] = raw
	s.Type = "kubernetes.io/dockerconfigjson"

	// Update the secret. The knative/pkg reconciler will only update status, so
	// we need to do this ourselves.
	if _, err := r.client.Secrets(s.Namespace).Update(ctx, s, metav1.UpdateOptions{}); err != nil {
		logger.Errorf("Failed to update secret: %v", err)
		return err
	}

	// Check again before the token will expire.
	logger.Infof("Updated secret, will check again in %s", ttl-Buffer)
	r.enqueueAfter(s, ttl-Buffer)
	return nil
}

func checkToken(ctx context.Context, s *corev1.Secret) (updateIn time.Duration) {
	logger := logging.FromContext(ctx)

	raw := s.Data[".dockerconfigjson"]
	if len(raw) == 0 {
		logger.Errorf("Missing .dockerconfigjson")
		return 0
	}
	var cfg dockerConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		logger.Errorf("Failed to unmarshal .dockerconfigjson: %v", err)
		return 0
	}
	current := cfg.Auths[Registry].Auth
	if len(current) == 0 {
		logger.Errorf("Missing current token")
		return 0
	}

	user, pass, ok := bytes.Cut(current, []byte{':'})
	if !ok {
		logger.Errorf("Failed to parse current token")
		return 0
	}
	if string(user) != "_token" {
		logger.Errorf("Unexpected username in current token: %q", user)
		return 0
	}

	// Construct a verifier that only accepts tokens from our issuer.
	provider, err := oidc.NewProvider(ctx, Issuer)
	if err != nil {
		logger.Errorf("Failed to construct OIDC provider: %v", err)
		return 0
	}
	tok, err := provider.VerifierContext(ctx, &oidc.Config{ClientID: Registry}).Verify(ctx, string(pass))
	if err != nil {
		logger.Errorf("Failed to verify token: %v", err)
		return 0
	}
	return time.Until(tok.Expiry) - Buffer
}

func newToken(ctx context.Context, identity string) (string, error) {
	logger := logging.FromContext(ctx)

	// Get the controller's SA token.
	saToken, err := os.ReadFile("/var/run/chainguard/oidc/oidc-token")
	if err != nil {
		return "", fmt.Errorf("unable to read service account token: %w", err)
	}
	u := url.URL{
		Scheme: "https",
		Host:   Issuer,
		Path:   "/sts/exchange",
		RawQuery: url.Values{
			"aud":      {Registry},
			"identity": {identity},
			// TODO: only request the capabilities we need.
		}.Encode(),
	}
	logger.Infof("POST %s", u.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("unable to create STS request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+string(saToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("STS request failed: %w", err)
	}
	defer resp.Body.Close()
	all, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read STS response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("STS exchange failed (%d): %s", resp.StatusCode, all)
	}
	var tok struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(all, &tok); err != nil {
		return "", fmt.Errorf("unable to unmarshal STS response: %w", err)
	}
	return tok.Token, nil
}
