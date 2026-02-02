package middleware

import (
	"ranklogger/config"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

func GetTrustedIP(c *fiber.Ctx, cfg *config.Config) string {
	xffHeaders := c.GetReqHeaders()["X-Forwarded-For"]

	if len(xffHeaders) > 0 {
		// 最も外側（自分にリクエストを投げたプロキシ）が追加したものは
		// 配列の「最後」の要素
		lastHeader := xffHeaders[len(xffHeaders)-cfg.Server.ClientIpFromLast]

		// もし1つのヘッダー内にカンマ区切りで複数ある場合は、その一番右を取る
		ips := strings.Split(lastHeader, ",")
		return strings.TrimSpace(ips[len(ips)-1])
	}

	// プロキシを介していない場合は直接のIP
	return c.IP()
}

// GlobalLimit: 全ユーザー合計の制限用 (KeyGeneratorを固定文字列にする)
func GlobalLimit(cfg *config.Config, name string) fiber.Handler {
	ep_cfg := cfg.Limits.Endpoints[name]
	return limiter.New(limiter.Config{
		Max:        ep_cfg.Max,
		Expiration: time.Duration(ep_cfg.Expiration) * time.Second,
		KeyGenerator: func(c *fiber.Ctx) string {
			return "server-global-limit" // 全リクエストで共通のキー
		},
		LimiterMiddleware: limiter.SlidingWindow{},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "Server-wide rate limit reached"})
		},
	})
}

// UserLimit: ユーザー単位の制限用 (IPアドレスを識別子にする)
func UserLimit(cfg *config.Config) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        cfg.Limits.UserLimit,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return GetTrustedIP(c, cfg)
		},
		LimiterMiddleware: limiter.SlidingWindow{},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "Too many requests from your IP"})
		},
	})
}
