// Package config centralises every environment-driven knob. Nothing sensitive
// is hardcoded: secrets and connection strings come from the environment
// (RNF004), with development-only defaults that are safe to publish.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Service names accepted by the `-service` flag of cmd/orion.
const (
	ServiceGateway      = "gateway"
	ServiceNLP          = "nlp"
	ServiceAuth         = "auth"
	ServiceCallMgmt     = "callmgmt"
	ServiceNotification = "notification"
	ServiceAll          = "all"
	ServiceSeed         = "seed"
)

// Config is the fully resolved runtime configuration of one process.
type Config struct {
	Service string
	Env     string

	// Ports. In `all` mode every service listens on its own port inside the
	// same process, which mirrors the container topology used by compose.
	GatewayPort      int
	NLPPort          int
	AuthPort         int
	CallMgmtPort     int
	NotificationPort int

	// Internal base URLs the gateway uses to reach its peers.
	NLPURL          string
	AuthURL         string
	CallMgmtURL     string
	NotificationURL string

	PostgresURL string
	RedisURL    string
	KafkaBroker string

	JWTSecret     string
	JWTTTL        time.Duration
	BcryptCost    int
	AllowedOrigin []string

	// ConfidenceThreshold is the minimum NLU confidence to keep a conversation
	// automated. Below it the gateway triggers the human handoff (flow B).
	ConfidenceThreshold float64

	AnthropicAPIKey string
	AnthropicModel  string
	NLUTimeout      time.Duration

	// InternalTimeout / InternalRetries bound every service-to-service call so
	// one slow dependency cannot blow the 2s budget (RNF001, RNF007).
	InternalTimeout time.Duration
	InternalRetries int

	SeedOnBoot bool
}

// Load reads the configuration from the environment and validates it.
func Load(service string) (Config, error) {
	cfg := Config{
		Service: service,
		Env:     env("ORION_ENV", "development"),

		GatewayPort:      envInt("ORION_GATEWAY_PORT", 8000),
		NLPPort:          envInt("ORION_NLP_PORT", 8010),
		AuthPort:         envInt("ORION_AUTH_PORT", 8011),
		CallMgmtPort:     envInt("ORION_CALLMGMT_PORT", 8012),
		NotificationPort: envInt("ORION_NOTIFICATION_PORT", 8013),

		NLPURL:          env("ORION_NLP_URL", "http://localhost:8010"),
		AuthURL:         env("ORION_AUTH_URL", "http://localhost:8011"),
		CallMgmtURL:     env("ORION_CALLMGMT_URL", "http://localhost:8012"),
		NotificationURL: env("ORION_NOTIFICATION_URL", "http://localhost:8013"),

		PostgresURL: env("ORION_POSTGRES_URL", ""),
		RedisURL:    env("ORION_REDIS_URL", ""),
		KafkaBroker: env("ORION_KAFKA_BROKER", ""),

		JWTSecret:  env("ORION_JWT_SECRET", "orion-dev-secret-change-me"),
		JWTTTL:     envDuration("ORION_JWT_TTL", time.Hour),
		BcryptCost: envInt("ORION_BCRYPT_COST", 10),

		ConfidenceThreshold: envFloat("ORION_CONFIDENCE_THRESHOLD", 0.70),

		AnthropicAPIKey: env("ANTHROPIC_API_KEY", ""),
		AnthropicModel:  env("ORION_NLU_MODEL", "claude-opus-5"),
		NLUTimeout:      envDuration("ORION_NLU_TIMEOUT", 4*time.Second),

		InternalTimeout: envDuration("ORION_INTERNAL_TIMEOUT", 3*time.Second),
		InternalRetries: envInt("ORION_INTERNAL_RETRIES", 2),

		SeedOnBoot: envBool("ORION_SEED", true),
	}

	origins := env("ORION_ALLOWED_ORIGINS", "*")
	for _, item := range strings.Split(origins, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			cfg.AllowedOrigin = append(cfg.AllowedOrigin, trimmed)
		}
	}

	if cfg.ConfidenceThreshold <= 0 || cfg.ConfidenceThreshold > 1 {
		return cfg, fmt.Errorf("ORION_CONFIDENCE_THRESHOLD must be in (0,1], got %v", cfg.ConfidenceThreshold)
	}
	if cfg.IsProduction() && cfg.JWTSecret == "orion-dev-secret-change-me" {
		return cfg, fmt.Errorf("ORION_JWT_SECRET must be set when ORION_ENV=production")
	}
	if cfg.BcryptCost < 4 || cfg.BcryptCost > 15 {
		return cfg, fmt.Errorf("ORION_BCRYPT_COST must be between 4 and 15, got %d", cfg.BcryptCost)
	}
	return cfg, nil
}

// IsProduction reports whether the stricter production checks apply.
func (c Config) IsProduction() bool { return c.Env == "production" }

// UsePostgres reports whether a relational database was configured. When it is
// empty the services fall back to the in-memory repositories, which keeps the
// prototype runnable with zero external dependencies.
func (c Config) UsePostgres() bool { return c.PostgresURL != "" }

// UseRedis reports whether a Redis session store was configured.
func (c Config) UseRedis() bool { return c.RedisURL != "" }

// UseKafka reports whether an external event bus was configured. Without it the
// process uses an in-process bus, which only works in `all` mode.
func (c Config) UseKafka() bool { return c.KafkaBroker != "" }

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if value, err := strconv.Atoi(env(key, "")); err == nil {
		return value
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if value, err := strconv.ParseFloat(env(key, ""), 64); err == nil {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if value, err := strconv.ParseBool(env(key, "")); err == nil {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if value, err := time.ParseDuration(env(key, "")); err == nil {
		return value
	}
	return fallback
}
