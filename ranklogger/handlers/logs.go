package handlers

import (
	"database/sql"
	"fmt"
	"html"
	"time"

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

		// play_countタグのバリデーション追加
		validate.RegisterValidation("play_count", func(fl validator.FieldLevel) bool {
			return !cfg.Server.EnablePlayCount || fl.Field().Int() >= 1
		})

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

		// play_countの有効無効の設定に応じた動的な変更
		reqId := []interface{}{req.UUID}
		playCountAddText := [3]string{}
		if cfg.Server.EnablePlayCount {
			reqId = append(reqId, req.PlayCount)
			playCountAddText[0] = ", play_count"
			playCountAddText[1] = ", ?"
			playCountAddText[2] = " AND play_count = ?"
		}
		query := fmt.Sprintf(`
			INSERT OR IGNORE INTO sessions (uuid%s, data, ip_address) 
			VALUES (?%s, jsonb('{}'), ?)`, playCountAddText[0], playCountAddText[1])
		_, err = tx.Exec(query, append(reqId, middleware.GetTrustedIP(c, cfg))...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Session creation failed"})
		}

		var sessionId int
		query = fmt.Sprintf("SELECT id FROM sessions WHERE uuid = ?%s", playCountAddText[2])
		err = tx.QueryRow(query, reqId...).Scan(&sessionId)
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

		sinceStr := c.Query("since")
		if sinceStr != "" {
			// 文字列を time.Time に変換
			sinceTime, err := time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "invalid since format (use RFC3339)"})
			}
			query += " AND created_at >= ?"
			args = append(args, sinceTime)
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
