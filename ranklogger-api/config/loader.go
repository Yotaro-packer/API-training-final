package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type (
	Config struct {
		Server          ServerConfig    `mapstructure:"server"`
		Auth            AuthConfig      `mapstructure:"auth"`
		Retention       RetentionConfig `mapstructure:"retention"`
		Limits          RateLimitConfig `mapstructure:"rate_limits"`
		Schema          []SchemaConfig  `mapstructure:"record_schema"`
		SortableColumns []SortOption    `json:"sortable_columns"`
	}

	ServerConfig struct {
		Port             int      `mapstructure:"port"`
		DBPath           string   `mapstructure:"db_path"`
		ReadTimeout      int      `mapstructure:"read_timeout"`
		ReadLimit        int      `mapstructure:"read_limit"`
		CORSAllowOrigins []string `mapstructure:"cors_allow_origins"`
	}

	AuthConfig struct {
		GameAPIKey        string `mapstructure:"game_api_key"`
		AdminUser         string `mapstructure:"admin_user"`
		AdminPasswordHash string `mapstructure:"admin_password_hash"`
	}

	RetentionConfig struct {
		LogRetentionDays int `mapstructure:"log_retention_days"`
	}

	RateLimitConfig struct {
		UserLimit int            `mapstructure:"user_limit"`
		Endpoints map[string]int `mapstructure:"endpoints"`
	}

	SchemaConfig struct {
		Name    string `mapstructure:"name"`
		Type    string `mapstructure:"type"`
		IsIndex bool   `mapstructure:"is_index"`
		Order   string `mapstructure:"order"`
		Min     *int   `mapstructure:"min"`
		Max     *int   `mapstructure:"max"`
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
