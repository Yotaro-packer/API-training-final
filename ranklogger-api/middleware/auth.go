package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
	"github.com/gofiber/fiber/v2/middleware/keyauth"
	"golang.org/x/crypto/bcrypt"
	"ranklogger-api/config"
)

// GameClientAuth: APIキー (X-Game-Key) による認証
func GameClientAuth(cfg *config.Config) fiber.Handler {
	return keyauth.New(keyauth.Config{
		KeyLookup: "header:X-Game-Key",
		Validator: func(c *fiber.Ctx, key string) (bool, error) {
			return key == cfg.Auth.GameAPIKey, nil
		},
	})
}

// AdminAuth: Basic認証 (Username + Hashed Password)
func AdminAuth(cfg *config.Config) fiber.Handler {
	return basicauth.New(basicauth.Config{
		Users: map[string]string{
			cfg.Auth.AdminUser: "", // 値は空にして、Authorizerでハッシュ照合する
		},
		Authorizer: func(user, pass string) bool {
			// ユーザー名の確認
			if user != cfg.Auth.AdminUser {
				return false
			}
			// パスワードのハッシュ照合
			err := bcrypt.CompareHashAndPassword([]byte(cfg.Auth.AdminPasswordHash), []byte(pass))
			return err == nil
		},
	})
}
