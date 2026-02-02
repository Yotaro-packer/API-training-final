package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type (
	Config struct {
		Server          ServerConfig    `mapstructure:"SERVER"`
		Auth            AuthConfig      `mapstructure:"AUTH"`
		Retention       RetentionConfig `mapstructure:"RETENTION"`
		Limits          RateLimitConfig `mapstructure:"RATE_LIMITS"`
		Schema          []SchemaConfig  `mapstructure:"RECORD_SCHEMA"`
		SortableColumns []SortOption    `json:"sortable_columns"`
	}

	ServerConfig struct {
		Port             int      `mapstructure:"PORT"`
		DBPath           string   `mapstructure:"DB_PATH"`
		ReadTimeout      int      `mapstructure:"READ_TIMEOUT"`
		ReadLimit        int      `mapstructure:"READ_LIMIT"`
		CORSAllowOrigins []string `mapstructure:"CORS_ALLOW_ORIGINS"`
		ClientIpFromLast int      `mapstructure:"CLIENT_IP_FROM_LAST"`
	}

	AuthConfig struct {
		GameAPIKey        string `mapstructure:"GAME_API_KEY"`
		AdminUser         string `mapstructure:"ADMIN_USER"`
		AdminPasswordHash string `mapstructure:"ADMIN_PASSWORD_HASH"`
	}

	RetentionConfig struct {
		LogRetentionDays int `mapstructure:"LOG_RETENTION_DAYS"`
	}

	RateLimitConfig struct {
		UserLimit int                            `mapstructure:"USER_LIMIT"`
		Endpoints map[string]EndpointLimitConfig `mapstructure:"ENDPOINTS"`
	}

	EndpointLimitConfig struct {
		Max        int `mapstructure:"MAX"`
		Expiration int `mapstructure:"EXPIRATION_SECONDS"`
	}

	SchemaConfig struct {
		Name    string `mapstructure:"NAME"`
		Type    string `mapstructure:"TYPE"`
		IsIndex bool   `mapstructure:"IS_INDEX"`
		Order   string `mapstructure:"ORDER"`
		Min     *int   `mapstructure:"MIN"`
		Max     *int   `mapstructure:"MAX"`
	}

	SortOption struct {
		Name  string `json:"name"`
		Order string `json:"order"`
	}
)

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
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	if cfg.Auth.GameAPIKey == "your-game-client-secret-key" {
		return nil, fmt.Errorf("APIキーを初期値から変更してください。\nconfig/config.yamlの中のauth:game_api_keyを確認してください。")
	}
	if cfg.Auth.AdminPasswordHash == "" {
		return nil, fmt.Errorf("管理者パスワードの設定をしてください。\nconfig/config.yamlの中のauth:admin_password_hashを確認してください。")
	}

	for _, field := range cfg.Schema {
		if field.IsIndex {
			// デフォルトの順序を設定（設定になければDESCにする）
			order := strings.ToUpper(field.Order)
			if order == "" {
				order = "DESC"
			}
			cfg.SortableColumns = append(cfg.SortableColumns, SortOption{
				Name:  field.Name,
				Order: order,
			})
		}
	}

	return &cfg, nil
}
