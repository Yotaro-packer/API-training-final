package handlers

import (
	"database/sql"
	"fmt"
	"html"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"

	"ranklogger/config"
	"ranklogger/middleware"
	"ranklogger/models"
)

var varidate = validator.New()

func PostLogs(db *sql.DB, cfg *config.Config) fiber.Handler {
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
			req.UUID, req.PlayCount, middleware.GetTrustedIP(c, cfg),
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

func GetLogs(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sessionId := c.Params("SessionId")

		// ページネーション設定
		limit := c.QueryInt("limit", 100) // ログは一度に多く見たいのでデフォルト100
		offset := c.QueryInt("offset", 0)
		logType := c.QueryInt("type", 0) // 0は指定なし

		// クエリの組み立て
		query := "SELECT id, type, content FROM logs WHERE session_id = ?"
		args := []interface{}{sessionId}

		if logType != 0 {
			query += " AND type = ?"
			args = append(args, logType)
		}

		query += fmt.Sprintf(" ORDER BY id ASC LIMIT %d OFFSET %d", limit, offset)

		// 実行
		rows, err := db.Query(query, args...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch logs"})
		}
		defer rows.Close()

		var logs []fiber.Map
		for rows.Next() {
			var id, lType int
			var content string
			if err := rows.Scan(&id, &lType, &content); err != nil {
				continue
			}
			logs = append(logs, fiber.Map{
				"id":      id,
				"type":    lType,
				"content": content, // すでに保存時にエスケープ済み
			})
		}

		return c.JSON(fiber.Map{
			"session_id": sessionId,
			"logs":       logs,
		})
	}
}
