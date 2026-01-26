package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Auth      AuthConfig      `mapstructure:"auth"`
	Retention RetentionConfig `mapstructure:"retention"`
	Limits    RateLimitConfig `mapstructure:"rate_limits"`
	Schema    []SchemaConfig  `mapstructure:"record_schema"`
}

type ServerConfig struct {
	Port             int      `mapstructure:"port"`
	DBPath           string   `mapstructure:"db_path"`
	CORSAllowOrigins []string `mapstructure:"cors_allow_origins"`
}

type AuthConfig struct {
	GameAPIKey        string `mapstructure:"game_api_key"`
	AdminUser         string `mapstructure:"admin_user"`
	AdminPasswordHash string `mapstructure:"admin_password_hash"`
}

type RetentionConfig struct {
	LogRetentionDays int `mapstructure:"log_retention_days"`
}

type RateLimitConfig struct {
	UserLimit int            `mapstructure:"user_limit"`
	Endpoints map[string]int `mapstructure:"endpoints"`
}

type SchemaConfig struct {
	Name    string `mapstructure:"name"`
	Type    string `mapstructure:"type"`
	IsIndex bool   `mapstructure:"is_index"`
	Order   string `mapstructure:"order"`
}

func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.SetConfigType("yaml")

	// デフォルト値の設定
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("retention.log_retention_days", 0)

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	err := viper.Unmarshal(&cfg)
	return &cfg, err
}
