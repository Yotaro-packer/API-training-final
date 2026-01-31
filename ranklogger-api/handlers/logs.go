package handlers

import (
	"database/sql"
	"html"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"

	"ranklogger-api/middleware"
	"ranklogger-api/models"
)

var varidate = validator.New()

func PostLogs(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req models.PostLogsRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
		}

		// バリデーション
		if err := validate.Struct(req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		// HTMLエスケープの適用
		for i := range req.Logs {
			req.Logs[i].Content = html.EscapeString(req.Logs[i].Content)
		}

		// トランザクション開始
		tx, err := db.Begin()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Transaction failed"})
		}
		defer tx.Rollback()

		// 1. セッションIDを取得、なければ作成 (INSERT OR IGNORE + SELECT)
		// dataは初期状態なので空のJSONBを入れる
		_, err = tx.Exec(`
			INSERT OR IGNORE INTO sessions (uuid, play_count, data, ip_address) 
			VALUES (?, ?, jsonb('{}'), ?)`,
			req.UUID, req.PlayCount, middleware.GetTrustedIP(c),
		)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Session creation failed"})
		}

		var sessionId int
		err = tx.QueryRow("SELECT id FROM sessions WHERE uuid = ? AND play_count = ?",
			req.UUID, req.PlayCount).Scan(&sessionId)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Session lookup failed"})
		}

		// 2. ログのバルクインサート
		// SQLiteの効率的なバルクインサート用にクエリを組み立てる
		if len(req.Logs) > 0 {
			query := "INSERT INTO logs (session_id, type, content) VALUES "
			vals := []interface{}{}
			for _, l := range req.Logs {
				query += "(?, ?, ?),"
				vals = append(vals, sessionId, l.Type, l.Content)
			}
			query = query[0 : len(query)-1] // 最後のカンマを削除

			stmt, err := tx.Prepare(query)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Prepare statement failed"})
			}
			if _, err := stmt.Exec(vals...); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Log insert failed"})
			}
		}

		if err := tx.Commit(); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Commit failed"})
		}

		return c.Status(201).JSON(fiber.Map{
			"message":    "Logs registered successfully",
			"session_id": sessionId,
		})
	}
}
