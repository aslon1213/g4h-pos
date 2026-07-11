package configs

import (
	"crypto/rand"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

var ENVT_TYPE_LOGGED bool = false

type Config struct {
	DB     DBConfig     `mapstructure:"database"`
	Redis  RedisConfig  `mapstructure:"redis"`
	Server ServerConfig `mapstructure:"server"`
	S3     S3Config     `mapstructure:"s3"`
}

type DBConfig struct {
	Host           string `mapstructure:"host"`
	Port           string `mapstructure:"port"`
	Username       string `mapstructure:"username"`
	Password       string `mapstructure:"password"`
	Database       string `mapstructure:"database"`
	MaxConnections uint64 `mapstructure:"max_connections"`
	MinPoolSize    uint64 `mapstructure:"min_pool_size"`
	Auth           bool   `mapstructure:"auth"`
	ReplicaSet     string `mapstructure:"replica_set"`
	URL            string `mapstructure:"url"`

	// PostgreSQL (Supabase) connection settings. DSN is the full connection
	// string (Supabase "connection pooler" URI for the app). When DSN is empty
	// it is assembled from host/port/username/password/database/ssl_mode.
	DSN     string `mapstructure:"dsn"`
	SSLMode string `mapstructure:"ssl_mode"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	Password string `mapstructure:"password"`
	Database int    `mapstructure:"database"`
}

type ProxyConfig struct {
	Type   string `mapstructure:"type"`
	Path   string `mapstructure:"path"`
	Addr   string `mapstructure:"addr"`
	APIKey string `mapstructure:"api_key"`
}

type ServerConfig struct {
	Host                    string          `mapstructure:"host"`
	Port                    string          `mapstructure:"port"`
	SecretSymmetricKey      string          `mapstructure:"secret_symmetric_key"`
	TokenExpiryHours        int             `mapstructure:"token_expiry_hours"`         // access token lifetime
	RefreshTokenExpiryHours int             `mapstructure:"refresh_token_expiry_hours"` // refresh token lifetime (defaults to 720h/30d if unset)
	AdminDocsUsers          []AdminDocsUser `mapstructure:"admin_docs_users"`
	Proxy                   []ProxyConfig   `mapstructure:"proxy"`
}

type S3Config struct {
	Region          string `mapstructure:"region"`
	Endpoint        string `mapstructure:"endpoint"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	ImageBucket     string `mapstructure:"image_bucket"`
}

type AdminDocsUser struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

func LoadConfig(path string) (*Config, error) {
	filename := "config"
	if strings.ToLower(os.Getenv("ENVIRONMENT")) != "production" {
		if os.Getenv("CONFIG_FILE") == "" {
			filename = "config.local"
		} else {
			filename = os.Getenv("CONFIG_FILE")
		}

	}
	if !ENVT_TYPE_LOGGED {
		log.Info().Str("ENVIRONMENT", os.Getenv("ENVIRONMENT")).Str("filename", filename).Msg("Loading config from YAML")
		ENVT_TYPE_LOGGED = true
	}

	if os.Getenv("LOAD_DOT_ENV") != "" {
		godotenv.Load()

	}

	log.Info().Msg("Loading Config from " + filename)

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_")) // to pass env variables with
	viper.SetConfigName(filename)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(path)
	viper.AddConfigPath("../")
	viper.AddConfigPath("../../")

	// Explicit env bindings — this is the only reliable way
	bindings := map[string]string{
		"database.host":                     "DATABASE_HOST",
		"database.port":                     "DATABASE_PORT",
		"database.username":                 "DATABASE_USERNAME",
		"database.password":                 "DATABASE_PASSWORD",
		"database.database":                 "DATABASE_NAME",
		"database.max_connections":          "DATABASE_MAX_CONNECTIONS",
		"database.min_pool_size":            "DATABASE_MIN_POOL_SIZE",
		"database.auth":                     "DATABASE_AUTH",
		"database.replica_set":              "DATABASE_REPLICA_SET",
		"database.url":                      "DATABASE_URL",
		"database.dsn":                      "DATABASE_DSN",
		"database.ssl_mode":                 "DATABASE_SSL_MODE",
		"s3.region":                         "S3_REGION",
		"s3.endpoint":                       "S3_ENDPOINT",
		"s3.access_key_id":                  "S3_ACCESS_KEY_ID",
		"s3.secret_access_key":              "S3_SECRET_ACCESS_KEY",
		"s3.image_bucket":                   "S3_IMAGE_BUCKET",
		"redis.host":                        "REDIS_HOST",
		"redis.port":                        "REDIS_PORT",
		"redis.password":                    "REDIS_PASSWORD",
		"redis.database":                    "REDIS_DATABASE",
		"server.host":                       "SERVER_HOST",
		"server.port":                       "SERVER_PORT",
		"server.secret_symmetric_key":       "SERVER_SECRET_SYMMETRIC_KEY",
		"server.token_expiry_hours":         "SERVER_TOKEN_EXPIRY_HOURS",
		"server.refresh_token_expiry_hours": "SERVER_REFRESH_TOKEN_EXPIRY_HOURS",
	}

	for key, env := range bindings {
		viper.BindEnv(key, env)
	}

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	// log environment with secrets redacted; the full (unredacted) config is only
	// logged when LOG_ENV=true, for local debugging.
	log.Info().Interface("config", redactedConfig(config)).Msg("Config loaded")
	if strings.ToLower(os.Getenv("LOG_ENV")) == "true" {
		log.Info().Interface("config", config).Msg("Config loaded (unredacted, LOG_ENV=true)")
	}

	return &config, nil
}

// redactedConfig returns a copy of cfg with credential-bearing fields masked so
// the config can be logged without leaking the DB URL/DSN, symmetric key, etc.
func redactedConfig(cfg Config) Config {
	c := cfg // shallow copy; we overwrite the sensitive scalar fields below
	c.DB.URL = mask(cfg.DB.URL)
	c.DB.DSN = mask(cfg.DB.DSN)
	c.DB.Password = mask(cfg.DB.Password)
	c.Server.SecretSymmetricKey = mask(cfg.Server.SecretSymmetricKey)
	c.Redis.Password = mask(cfg.Redis.Password)
	c.S3.SecretAccessKey = mask(cfg.S3.SecretAccessKey)

	c.Server.Proxy = maskProxies(cfg.Server.Proxy) // ProxyConfig carries api keys
	c.Server.AdminDocsUsers = maskAdminUsers(cfg.Server.AdminDocsUsers)
	return c
}

func mask(s string) string {
	if s == "" {
		return ""
	}
	return "***REDACTED***"
}

func maskProxies(in []ProxyConfig) []ProxyConfig {
	out := make([]ProxyConfig, len(in))
	for i, p := range in {
		p.APIKey = mask(p.APIKey)
		out[i] = p
	}
	return out
}

func maskAdminUsers(in []AdminDocsUser) []AdminDocsUser {
	out := make([]AdminDocsUser, len(in))
	for i, u := range in {
		u.Password = mask(u.Password)
		out[i] = u
	}
	return out
}

func GenerateSecretSymmetricKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		log.Fatal().Err(err).Msg("Failed to generate symmetric key")
	}
	log.Info().Str("secret_symmetric_key", string(key)).Int("Length", len(key)).Msg("secret_symmetric_key")
	return key
}
