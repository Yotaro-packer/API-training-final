package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

func GetTrustedIP(c *fiber.Ctx) string {
	xffHeaders := c.GetReqHeaders()["X-Forwarded-For"]

	if len(xffHeaders) > 0 {
		// 最も外側（自分にリクエストを投げたプロキシ）が追加したものは
		// 配列の「最後」の要素
		lastHeader := xffHeaders[len(xffHeaders)-1]

		// もし1つのヘッダー内にカンマ区切りで複数ある場合は、その一番右を取る
		ips := strings.Split(lastHeader, ",")
		return strings.TrimSpace(ips[len(ips)-1])
	}

	// プロキシを介していない場合は直接のIP
	return c.IP()
}
