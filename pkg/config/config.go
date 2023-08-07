package config

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/logging"
)

type Store struct {
	*configmap.UntypedStore
}

// NewStore creates a new store of Configs and optionally calls functions when ConfigMaps are updated.
func NewStore(ctx context.Context) *Store {
	store := &Store{
		UntypedStore: configmap.NewUntypedStore(
			"config",
			logging.FromContext(ctx).Named("config-store"),
			configmap.Constructors{
				"config": newConfigFromConfigMap,
			},
		),
	}

	return store
}

type Config struct {
	Audience string        `json:"audience"`
	Issuer   string        `json:"issuer"`
	Buffer   time.Duration `json:"buffer"`
}

func defaultConfig() *Config {
	return &Config{
		Audience: "cgr.dev",
		Issuer:   "issuer.enforce.dev",
		Buffer:   10 * time.Minute,
	}
}

// newConfigFromConfigMap returns a Config for the given configmap
func newConfigFromConfigMap(config *corev1.ConfigMap) (*Config, error) {
	cfg := defaultConfig()

	if config.Data["audience"] != "" {
		cfg.Audience = config.Data["audience"]
	}
	if config.Data["issuer"] != "" {
		cfg.Issuer = config.Data["issuer"]
	}

	if config.Data["buffer"] != "" {
		d, err := time.ParseDuration(config.Data["buffer"])
		if err != nil {
			return nil, err
		}
		cfg.Buffer = d
	}
	return cfg, nil
}

type cfgKey struct{}

// ToContext attaches the provided Config to the provided context, returning the
// new context with the Config attached.
func ToContext(ctx context.Context, c *Config) context.Context {
	return context.WithValue(ctx, cfgKey{}, c)
}

func FromContext(ctx context.Context) *Config {
	if c := ctx.Value(cfgKey{}); c == nil {
		return defaultConfig()
	} else {
		return c.(*Config)
	}
}

// Load creates a Config from the current config state of the Store.
func (s *Store) Load() *Config {
	config := s.UntypedLoad("config")
	if config == nil {
		config = defaultConfig()
	}
	return config.(*Config)
}
