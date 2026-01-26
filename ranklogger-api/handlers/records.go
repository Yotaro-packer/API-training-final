package handlers

import (
	"database/sql"
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	"ranklogger-api/config"
	"ranklogger-api/models"
)

// PostRecord はスコアを保存するハンドラー
func PostRecord(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 1. JSONの解析
		var input models.GameRecordInput
		if err := c.BodyParser(&input); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "JSONの形式が正しくありません"})
		}

		// 2. dataフィールド（map）を文字列（JSON）に変換してDBに保存できるようにする
		jsonData, _ := json.Marshal(input.Data)

		// 3. SQLiteの UPSERT (INSERT ... ON CONFLICT)
		// uuid と play_count の組み合わせが重複していたら data を更新する
		query := `
			INSERT INTO sessions (uuid, play_count, data)
			VALUES (?, ?, jsonb(?))
			ON CONFLICT(uuid, play_count) DO UPDATE SET
				data = jsonb(excluded.data),
				created_at = CURRENT_TIMESTAMP;
		`

		_, err := db.Exec(query, input.UUID, input.PlayCount, string(jsonData))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "データベースへの保存に失敗しました"})
		}

		return c.Status(201).JSON(fiber.Map{"message": "記録が保存されました"})
	}
}

// handlers/records.go に追加
func GetRecords(db *sql.DB, cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 今回は例として、score（大きい順）で取得してみる
		// ※本来はクエリパラメータ等でソート項目を切り替えられるようにします
		query := "SELECT uuid, score, (data ->> '$.display_name') AS display_name FROM sessions ORDER BY score DESC LIMIT 10"

		rows, err := db.Query(query)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		var results []fiber.Map
		for rows.Next() {
			var uuid, name string
			var score int
			rows.Scan(&uuid, &score, &name)
			results = append(results, fiber.Map{
				"uuid":         uuid,
				"score":        score,
				"display_name": name,
			})
		}

		return c.JSON(results)
	}
}
