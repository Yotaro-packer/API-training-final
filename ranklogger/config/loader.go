package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type (
	Config struct {
		Server          ServerConfig            `mapstructure:"SERVER"`
		Auth            AuthConfig              `mapstructure:"AUTH"`
		Retention       RetentionConfig         `mapstructure:"RETENTION"`
		Limits          RateLimitConfig         `mapstructure:"RATE_LIMITS"`
		Schema          []SchemaConfig          `mapstructure:"RECORD_SCHEMA"`
		SortableColumns map[string][]SortOption `json:"sortable_columns"`
		TypeValidation  TypeConfig              `mapstructure:"TYPE_VALIDATION"`
	}

	ServerConfig struct {
		Port             int      `mapstructure:"PORT"`
		APIRootPath      string   `mapstructure:"API_ROOT_PATH"`
		DBPath           string   `mapstructure:"DB_PATH"`
		ReadTimeout      int      `mapstructure:"READ_TIMEOUT"`
		ReadLimit        int      `mapstructure:"READ_LIMIT"`
		CORSAllowOrigins []string `mapstructure:"CORS_ALLOW_ORIGINS"`
		ClientIpFromLast int      `mapstructure:"CLIENT_IP_FROM_LAST"`
		EnablePlayCount  bool     `mapstructure:"ENABLE_PLAY_COUNT"`
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
		Name     string `mapstructure:"NAME"`
		Type     string `mapstructure:"TYPE"`
		IsIndex  bool   `mapstructure:"IS_INDEX"`
		Order    string `mapstructure:"ORDER"`
		Min      *int   `mapstructure:"MIN"`
		Max      *int   `mapstructure:"MAX"`
		Tag      string `mapstructure:"TAG"`
		IsGlobal bool   `mapstructure:"IS_GLOBAL"`
	}

	SortOption struct {
		Name  string `json:"name"`
		Order string `json:"order"`
	}

	TypeConfig struct {
		Strings []string `mapstructure:"STRINGS"`
		Numbers []string `mapstructure:"NUMBERS"`
	}
)

func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.SetConfigType("yaml")

	// デフォルト値の設定
	viper.SetDefault("SERVER.PORT", 8080)
	viper.SetDefault("SERVER.DB_PATH", "./data/game_data.db")
	viper.SetDefault("SERVER.READ_TIMEOUT", 10)
	viper.SetDefault("SERVER.READ_LIMIT", 100)
	viper.SetDefault("SERVER.CORS_ALLOW_ORIGINS", "*")
	viper.SetDefault("SERVER.CLIENT_IP_FROM_LAST", 1)
	// 0やfalseをデフォルト値にする項目はUnmarshalで設定される

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	if cfg.Auth.GameAPIKey == "your-game-client-secret-key" {
		return nil, fmt.Errorf("APIキーを初期値から変更してください。\nconfig.yamlの中のAUTH:GAME_API_KEYを確認してください。")
	}
	if cfg.Auth.AdminPasswordHash == "" {
		return nil, fmt.Errorf("管理者パスワードの設定をしてください。\nconfig.yamlの中のAUTH:ADMIN_PASSWORD_HASHを確認してください。")
	}

	cfg.SortableColumns = make(map[string][]SortOption)
	for _, field := range cfg.Schema {
		if field.Name == "" {
			return nil, fmt.Errorf("名前が設定されていないレコードスキーマがあります。\nconfig.yamlの中のRECORD_SCHEMAを確認してください。")
		}
		if lowerTag := strings.ToLower(field.Tag); lowerTag == "detail" || lowerTag == "global" || lowerTag == "none" {
			return nil, fmt.Errorf("%s はタグ名に使用できません。 \nconfig.yamlの中のRECORD_SCHEMAを確認してください。", field.Tag)
		}
		if field.IsIndex {
			// デフォルトの順序を設定（設定になければDESCにする）
			order := strings.ToUpper(field.Order)
			if order == "" {
				order = "DESC"
			}
			if field.Tag == "" {
				if field.IsGlobal {
					return nil, fmt.Errorf("%sのタグを設定するかIsGlobalをFalseにしてください。\nconfig.yamlの中のRECORD_SCHEMAを確認してください。", field.Name)
				}
				cfg.SortableColumns["none"] = append(cfg.SortableColumns["none"], SortOption{
					Name:  field.Name,
					Order: order,
				})
				cfg.SortableColumns["global"] = append(cfg.SortableColumns["global"], SortOption{
					Name:  field.Name,
					Order: order,
				})
			} else {
				cfg.SortableColumns[field.Tag] = append(cfg.SortableColumns[field.Tag], SortOption{
					Name:  field.Name,
					Order: order,
				})
				if field.IsGlobal {
					cfg.SortableColumns["global"] = append(cfg.SortableColumns["global"], SortOption{
						Name:  field.Tag + "_" + field.Name,
						Order: order,
					})
				}
			}
		}
	}

	return &cfg, nil
}
