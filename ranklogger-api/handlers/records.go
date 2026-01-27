package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"

	"ranklogger-api/config"
	"ranklogger-api/models"
)

var validate = validator.New()

// PostRecord はスコアを保存するハンドラー
func PostRecord(db *sql.DB, cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// JSONの解析
		var input models.GameRecordInput
		if err := c.BodyParser(&input); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "JSONの形式が正しくありません"})
		}
		// 基本構造のバリデーション (Fiberガイドの手法)
		if err := validate.Struct(input); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		// 動的スキーマチェック (config.yamlとの照合)
		for _, field := range cfg.Schema {
			val, exists := input.Data[field.Name]
			if !exists {
				return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Missing required field: %s", field.Name)})
			}

			// 型の簡易チェック (例: INTEGERならfloat64としてパースされるので数値チェック)
			switch field.Type {
			case "INTEGER", "FLOAT":
				num, ok := val.(float64) // JSONの数値はGoではfloat64としてパースされる
				if !ok {
					return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("%s must be a number", field.Name)})
				}

				// min / max チェック
				if field.Min != nil && int(num) < *field.Min {
					return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("%s must be >= %d", field.Name, *field.Min)})
				}
				if field.Max != nil && int(num) > *field.Max {
					return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("%s must be <= %d", field.Name, *field.Max)})
				}

			case "TEXT":
				str, ok := val.(string)
				if !ok {
					return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("%s must be a string", field.Name)})
				}

				// min / max チェック
				if field.Min != nil && len([]rune(str)) < *field.Min {
					return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("length of %s must be >= %d", field.Name, *field.Min)})
				}
				if field.Max != nil && len([]rune(str)) > *field.Max {
					return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("length of %s must be <= %d", field.Name, *field.Max)})
				}

				// --- XSS対策: HTMLエスケープ ---
				input.Data[field.Name] = html.EscapeString(str)
			}
		}

		// dataフィールド（map）を文字列（JSON）に変換してDBに保存できるようにする
		jsonData, _ := json.Marshal(input.Data)

		// SQLiteの UPSERT (INSERT ... ON CONFLICT)
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
		// デフォルト値
		limit := 10
		offset := 0

		// ランキング有効チェック
		if len(cfg.SortableColumns) == 0 {
			return c.JSON(fiber.Map{
				"message": "Ranking is disabled",
				"data":    []interface{}{},
			})
		}

		// sort_by の取得と検証
		sortBy := c.Query("sort_by", cfg.SortableColumns[0].Name)

		var currentSort config.SortOption
		found := false
		for _, opt := range cfg.SortableColumns {
			if opt.Name == sortBy {
				currentSort = opt
				found = true
				break
			}
		}

		if !found {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid sort key"})
		}

		// 設定ファイルの record_schema にある全ての項目を抽出対象にする
		var selectColumns []string
		for _, field := range cfg.Schema {
			col := fmt.Sprintf("(data ->> '$.%s') AS %s", field.Name, field.Name)
			selectColumns = append(selectColumns, col)
		}
		// カンマで連結
		selection := strings.Join(selectColumns, ", ")

		isReverse, _ := strconv.ParseBool(c.Query("is_reverse"))
		finalOrder := currentSort.Order
		if isReverse {
			if finalOrder == "DESC" {
				finalOrder = "ASC"
			} else {
				finalOrder = "DESC"
			}
		}

		//limit, offsetの取得
		limitStr := c.Query("limit")
		if limitStr != "" {
			val, err := strconv.Atoi(limitStr)
			if err != nil || val < 0 {
				return c.Status(400).JSON(fiber.Map{"error": "limit must be a positive integer"})
			}
			limit = val
		}

		offsetStr := c.Query("offset")
		if offsetStr != "" {
			val, err := strconv.Atoi(offsetStr)
			if err != nil || val < 0 {
				return c.Status(400).JSON(fiber.Map{"error": "offset must be a positive integer"})
			}
			offset = val
		}

		// クエリの組み立て
		query := fmt.Sprintf(
			"SELECT %s FROM sessions ORDER BY %s %s NULLS LAST LIMIT %d OFFSET %d",
			selection, currentSort.Name, finalOrder, limit, offset,
		)

		rows, err := db.Query(query)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		// レスポンスの成形
		// カラム名の一覧を取得
		cols, _ := rows.Columns()
		var results []fiber.Map
		for rows.Next() {
			// データの受け皿を動的に作成
			columns := make([]interface{}, len(cols))
			columnPointers := make([]interface{}, len(cols))
			for i := range columns {
				columnPointers[i] = &columns[i]
			}

			// スキャン
			if err := rows.Scan(columnPointers...); err != nil {
				return err
			}

			// Mapに変換
			rowData := make(fiber.Map)
			for i, colName := range cols {
				rowData[colName] = columns[i]
			}
			results = append(results, rowData)
		}

		// レスポンスにメタデータを含める
		return c.JSON(fiber.Map{
			"meta": cfg.SortableColumns, // UI側はこの配列を見てタブやボタンを作れる
			"data": results,
		})
	}
}
