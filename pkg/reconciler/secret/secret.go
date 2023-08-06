/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package secret

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"

	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

const ( // TODO: configurable
	registry = "cgr.dev"
	issuer   = "issuer.enforce.dev"
	identity = "abc/def"
	repo     = "abc/ghi"
	ttl      = time.Hour
	buffer   = 10 * time.Minute

	labelKey = "pull-secret-updater.chainguard.dev/identity"
)

type Reconciler struct {
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
	logger.Infow("Reconciling")

	if s.Labels == nil || s.Labels[labelKey] == "" {
		logger.Debugw("Skipping",
			"reason", "missing identity label")
		return nil
	}
	if s.Labels[labelKey] != identity {
		logger.Debugw("Skipping",
			"reason", "not identity secret",
			"got", s.Labels["chainguard.dev/identity"],
			"want", identity)
		return nil
	}

	updateIn := checkToken(ctx, s)
	if updateIn > 0 {
		logger.Debugw("Enqueueing future refresh",
			"reason", "token valid and not expired",
			"updateIn", updateIn)
		r.enqueueAfter(s, updateIn)
		return nil
	}

	logger.Infof("Token needs update")

	// Get a new token.
	token, err := newToken(ctx)
	if err != nil {
		logger.Errorf("Failed to get new token: %w", err)
		return err
	}

	cfg := dockerConfig{
		Auths: map[string]dockerConfigAuth{
			registry: {Auth: []byte(base64.StdEncoding.EncodeToString([]byte("_token:" + token)))},
		},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		logger.Errorf("Failed to marshal .dockerconfigjson: %w", err)
		return err
	}
	s.Data[".dockerconfigjson"] = raw
	s.Type = "kubernetes.io/dockerconfigjson"

	// Check again before the token will expire.
	r.enqueueAfter(s, ttl-buffer)
	return nil
}

func checkToken(ctx context.Context, s *corev1.Secret) (updateIn time.Duration) {
	logger := logging.FromContext(ctx)

	raw := s.Data[".dockerconfigjson"]
	b, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		logger.Errorf("Failed to decode .dockerconfigjson: %w", err)
		return 0
	}
	var cfg dockerConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		logger.Errorf("Failed to unmarshal .dockerconfigjson: %w", err)
		return 0
	}
	current := cfg.Auths[registry].Auth

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
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		logger.Errorf("Failed to construct OIDC provider: %v", err)
		return 0
	}
	verifier := provider.VerifierContext(ctx, &oidc.Config{
		SkipClientIDCheck: true, // Checked in the token getter below
	})
	tok, err := verifier.Verify(ctx, string(pass))
	if err != nil {
		logger.Errorf("Failed to verify token: %v", err)
		return 0
	}
	if !slices.Contains(tok.Audience, registry) {
		logger.Errorf("Unexpected token audience: %v", tok.Audience)
		return 0
	}

	return time.Until(tok.Expiry) - buffer
}

func newToken(ctx context.Context) (string, error) {
	// Get the controller's SA token.
	saToken, err := os.ReadFile("/var/run/chainguard/oidc/oidc-token")
	if err != nil {
		return "", fmt.Errorf("unable to read service account token: %w", err)
	}
	u := url.URL{
		Scheme: "https",
		Host:   issuer,
		Path:   "/sts/exchange",
	}
	u.Query().Add("aud", registry)
	u.Query().Add("identity", identity)
	u.Query().Add("scope", "registry.pull")
	u.Query().Add("repo", repo)
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
		return "", fmt.Errorf("STS request failed (%d): %s", resp.StatusCode, all)
	}
	var tok struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(all, &tok); err != nil {
		return "", fmt.Errorf("unable to unmarshal STS response: %w", err)
	}
	return tok.Token, nil
}
