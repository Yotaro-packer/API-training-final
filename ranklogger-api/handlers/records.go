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
	"ranklogger-api/middleware"
	"ranklogger-api/models"
)

var validate = validator.New()

// PostRecord はスコアを保存するハンドラー
func PostRecord(db *sql.DB, cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// JSONの解析
		var input models.GameRecordInput
		if err := c.BodyParser(&input); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
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
			INSERT INTO sessions (uuid, play_count, data, ip_address)
			VALUES (?, ?, jsonb(?), ?)
			ON CONFLICT(uuid, play_count) DO UPDATE SET
				data = jsonb(excluded.data),
				created_at = CURRENT_TIMESTAMP;
		`

		result, err := db.Exec(query, input.UUID, input.PlayCount, string(jsonData), middleware.GetTrustedIP(c))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Record insert failed"})
		}

		// 挿入された行のIDを取得
		lastId, _ := result.LastInsertId()

		return c.Status(201).JSON(fiber.Map{
			"message":    "Record registered successfully",
			"session_id": lastId,
		})
	}
}

// handlers/records.go に追加
func GetRecords(db *sql.DB, cfg *config.Config, detail bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
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
		// /records/detailなら詳細情報を表示
		if detail {
			selectColumns = []string{"id", "uuid", "play_count", "ip_address", "disable", "created_at"}
		}

		for _, field := range cfg.Schema {
			col := fmt.Sprintf("(data ->> '$.%s') AS %s", field.Name, field.Name)
			selectColumns = append(selectColumns, col)
		}

		isReverse, _ := strconv.ParseBool(c.Query("is_reverse"))
		finalOrder := currentSort.Order
		if isReverse {
			if finalOrder == "DESC" {
				finalOrder = "ASC"
			} else {
				finalOrder = "DESC"
			}
		}

		// 順位の付与
		rankCol := fmt.Sprintf("RANK() OVER (ORDER BY %s %s) AS rank", sortBy, finalOrder)
		selectColumns = append([]string{rankCol}, selectColumns...)

		selection := strings.Join(selectColumns, ", ")

		// limit, offsetの取得
		limit := c.QueryInt("limit", 10)
		offset := c.QueryInt("offset", 0)

		if limit < 0 || offset < 0 {
			return c.Status(400).JSON(fiber.Map{"error": "limit and offset must be positive"})
		}

		// 最大値の制限（負荷対策）
		if limit > cfg.Server.ReadLimit {
			limit = cfg.Server.ReadLimit
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

func GetRanks(db *sql.DB, cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sessionId := c.Params("SessionId")
		sortBy := c.Query("sort_by", cfg.SortableColumns[0].Name)

		// 1. ソート対象のバリデーション
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

		// 2. ウィンドウ関数を使って特定のIDの順位を抽出するクエリ
		// CTE (WITH句) で全行に順位を振り、外側で特定のIDで絞り込む
		query := fmt.Sprintf(`
			WITH ranked_sessions AS (
				SELECT 
					id,
					RANK() OVER (ORDER BY %s %s) as current_rank
				FROM sessions
				WHERE disable = FALSE
			)
			SELECT current_rank FROM ranked_sessions WHERE id = ?`,
			currentSort.Name, currentSort.Order,
		)

		var rank int
		err := db.QueryRow(query, sessionId).Scan(&rank)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Session not found"})
		}

		return c.JSON(fiber.Map{
			"session_id": sessionId,
			"sort_by":    sortBy,
			"rank":       rank,
		})
	}
}
