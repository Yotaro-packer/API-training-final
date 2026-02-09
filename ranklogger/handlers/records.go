package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"

	"ranklogger/config"
	"ranklogger/middleware"
	"ranklogger/models"
)

var validate = validator.New()

// PostRecord はスコアを保存するハンドラー
func PostRecord(db *sql.DB, cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// JSONの解析
		var input models.GameRecordRequest
		if err := c.BodyParser(&input); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
		}

		// play_countタグのバリデーション追加
		validate.RegisterValidation("play_count", func(fl validator.FieldLevel) bool {
			return !cfg.Server.EnablePlayCount || fl.Field().Int() >= 1
		})

		// modelsのstructで設定したタグに応じたバリデーションを実施
		if err := validate.Struct(input); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		tag := c.Params("Tag")
		// 動的スキーマチェック (config.yamlとの照合)
		renamedData := make(map[string]interface{})
		for _, field := range cfg.Schema {
			// タグが設定されていてかつ異なったら無視
			if field.Tag != "" && field.Tag != tag {
				continue
			}

			val, exists := input.Data[field.Name]
			if !exists {
				return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Missing required field: %s", field.Name)})
			}

			// 型の簡易チェック (例: INTEGERならfloat64としてパースされるので数値チェック)
			switch true {
			case slices.Contains(cfg.TypeValidation.Numbers, field.Type):
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

			case slices.Contains(cfg.TypeValidation.Strings, field.Type):
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
			// タグ付きフィールドの名前変更
			if field.Tag != "" {
				renamedData[field.Tag+"_"+field.Name] = input.Data[field.Name]
			} else {
				renamedData[field.Name] = input.Data[field.Name]
			}
		}

		// dataフィールド（map）を文字列（JSON）に変換してDBに保存できるようにする
		jsonData, _ := json.Marshal(renamedData)

		// play_countの有効無効の設定に応じた動的な変更
		inputId := []interface{}{input.UUID}
		playCountAddText := [3]string{}
		if cfg.Server.EnablePlayCount {
			inputId = append(inputId, input.PlayCount)
			playCountAddText[0] = ", play_count"
			playCountAddText[1] = ", ?"
			playCountAddText[2] = ", play_count"
		}
		// SQLiteの UPSERT (INSERT ... ON CONFLICT)
		query := fmt.Sprintf(`
			INSERT INTO sessions (uuid%s, data, ip_address)
			VALUES (?%s, jsonb(?), ?)
			ON CONFLICT(uuid%s) DO UPDATE SET
				data = jsonb_patch(data, excluded.data),
				created_at = CURRENT_TIMESTAMP
			RETURNING id
		`, playCountAddText[0], playCountAddText[1], playCountAddText[2])

		var sessionId int64
		err := db.QueryRow(query, append(inputId, string(jsonData), middleware.GetTrustedIP(c, cfg))...).Scan(&sessionId)

		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Record insert failed"})
		}

		return c.Status(201).JSON(fiber.Map{
			"message":    "Record registered successfully",
			"session_id": sessionId,
		})
	}
}

// レコードの無効化・有効化
func DisableRecord(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sessionId := c.Params("SessionId")

		var req models.DisableRecordRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
		}

		if err := validate.Struct(req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		// データベース更新
		result, err := db.Exec("UPDATE sessions SET disable = ? WHERE id = ?", req.Disable, sessionId)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update record"})
		}

		// 該当レコードが存在したかチェック
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "Session not found"})
		}

		statusMsg := "enabled"
		if req.Disable {
			statusMsg = "disabled"
		}

		return c.JSON(fiber.Map{
			"message":    fmt.Sprintf("Record status updated to %s", statusMsg),
			"session_id": sessionId,
			"disable":    req.Disable,
		})
	}
}

// レコードの取得
func GetRecords(db *sql.DB, cfg *config.Config, detail bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// ランキング有効チェック
		if len(cfg.Schema) == 0 {
			return c.JSON(fiber.Map{
				"message": "Ranking is disabled",
				"data":    []interface{}{},
			})
		}
		tag := c.Params("Tag")
		if tag == "" {
			tag = "global"
		} else if len(cfg.SortableColumns[tag]) == 0 {
			return c.JSON(fiber.Map{
				"message": "This tag's ranking is disabled",
				"data":    []interface{}{},
			})
		}

		// sort_by の取得と検証
		sortBy := c.Query("sort_by", cfg.SortableColumns[tag][0].Name)

		var currentSort config.SortOption
		var sortKey []config.SortOption

		// 管理者用ソートキーの追加
		if detail {
			sortKey = append(sortKey,
				config.SortOption{Name: "id", Order: ""},
				config.SortOption{Name: "created_at", Order: ""},
			)
		}

		found := false
		if tag == "global" {
			for _, opt := range cfg.SortableColumns[tag] {
				if opt.Name == sortBy {
					currentSort = opt
					found = true
					break
				}
			}
		} else {
			// タグ指定があった場合は、そのタグ特有のソートキーを優先する
			for _, opt := range cfg.SortableColumns["none"] {
				if opt.Name == sortBy {
					currentSort = opt
					found = true
					break
				}
			}
			for _, opt := range cfg.SortableColumns[tag] {
				if opt.Name == sortBy {
					currentSort = opt
					found = true
					// タグ特有のソートキーの場合、DB上の名称に変換
					sortBy = tag + "_" + sortBy
					break
				}
			}
		}

		if !found {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid sort key"})
		}

		var selectColumns []string
		var where string
		// 管理者向けの/records/detailなら詳細情報を表示
		if detail {
			selectColumns = []string{"id", "uuid", "ip_address", "disable", "created_at"}
			if cfg.Server.EnablePlayCount {
				selectColumns = append(selectColumns, "play_count")
			}
		} else {
			where = "WHERE disable = FALSE"
		}

		sinceStr := c.Query("since")
		var args []interface{}

		// 期間絞り込みロジック
		if sinceStr != "" {
			// 文字列を time.Time に変換
			sinceTime, err := time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "invalid since format (use RFC3339)"})
			}
			if where == "" {
				where = "WHERE created_at >= ?"
			} else {
				where += " AND created_at >= ?"
			}
			args = append(args, sinceTime)
		}

		if tag == "global" {
			for _, field := range cfg.Schema {
				// タグ指定なしの場合、global := タグ無し + IsGlobalフラグ付きの列を取得
				if field.Tag != "" && !field.IsGlobal {
					continue
				}
				col := fmt.Sprintf("(data ->> '$.%s') AS %s", field.Name, field.Name)
				selectColumns = append(selectColumns, col)
			}
		} else {
			for _, field := range cfg.Schema {
				// タグ指定有りの場合、タグ無しとタグ一致の列を取得
				switch field.Tag {
				case "":
					col := fmt.Sprintf("(data ->> '$.%s') AS %s", field.Name, field.Name)
					selectColumns = append(selectColumns, col)
				case tag:
					col := fmt.Sprintf("(data ->> '$.%s') AS %s", tag+"_"+field.Name, field.Name)
					selectColumns = append(selectColumns, col)
				}
			}
		}

		isReverse, _ := strconv.ParseBool(c.Query("is_reverse"))
		finalOrder := currentSort.Order
		useRank := true
		if isReverse {
			switch finalOrder {
			case "DESC":
				finalOrder = "ASC"
			case "ASC":
				finalOrder = "DESC"
			default:
				// 管理者用ソートキーなど、ランクが必要なくASC固定
				finalOrder = "ASC"
				useRank = false
			}
		}

		// 順位の付与
		if useRank {
			rankCol := fmt.Sprintf("RANK() OVER (ORDER BY %s %s) AS rank", sortBy, finalOrder)
			selectColumns = append([]string{rankCol}, selectColumns...)
		}

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

		// クエリの組み立て
		query := fmt.Sprintf(
			"SELECT %s FROM sessions %s ORDER BY %s %s NULLS LAST LIMIT %d OFFSET %d",
			selection, where, sortBy, finalOrder, limit, offset,
		)

		rows, err := db.Query(query, args...)
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

		var sortOptions []config.SortOption
		if tag != "global" {
			sortOptions = cfg.SortableColumns["none"]
		}
		sortOptions = append(sortOptions, cfg.SortableColumns[tag]...)
		// レスポンスにメタデータを含める
		return c.JSON(fiber.Map{
			"meta": sortOptions, // UI側はこの配列を見てタブやボタンを作れる
			"data": results,
		})
	}
}

func GetRanks(db *sql.DB, cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sessionId := c.Params("SessionId")
		// ランキング有効チェック
		if len(cfg.Schema) == 0 {
			return c.JSON(fiber.Map{
				"message": "Ranking is disabled",
				"data":    []interface{}{},
			})
		}
		tag := c.Params("Tag")
		if tag == "" {
			tag = "global"
		} else if len(cfg.SortableColumns[tag]) == 0 {
			return c.JSON(fiber.Map{
				"message": "This tag's ranking is disabled",
				"data":    []interface{}{},
			})
		}

		sortBy := c.Query("sort_by", cfg.SortableColumns[tag][0].Name)

		// 1. ソート対象のバリデーション
		var currentSort config.SortOption
		found := false
		if tag == "global" {
			for _, opt := range cfg.SortableColumns[tag] {
				if opt.Name == sortBy {
					currentSort = opt
					found = true
					break
				}
			}
		} else {
			// タグ指定があった場合は、そのタグ特有のソートキーを優先する
			for _, opt := range cfg.SortableColumns["none"] {
				if opt.Name == sortBy {
					currentSort = opt
					found = true
					break
				}
			}
			for _, opt := range cfg.SortableColumns[tag] {
				if opt.Name == sortBy {
					currentSort = opt
					found = true
					// タグ特有のソートキーの場合、DB上の名称に変換
					sortBy = tag + "_" + sortBy
					break
				}
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
			sortBy, currentSort.Order,
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
