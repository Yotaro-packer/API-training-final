package handlers

import (
	"database/sql"
	"fmt"
	"ranklogger/middleware"
	"runtime"

	"github.com/gofiber/fiber/v2"
)

func GetMetrics(db *sql.DB, stats *middleware.LatencyStats) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 1. レコード総数の取得
		var totalSessions, totalLogs int
		_ = db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&totalSessions)
		_ = db.QueryRow("SELECT COUNT(*) FROM logs").Scan(&totalLogs)

		// 2. DBファイルサイズの取得 (SQLite特有)
		// ページ数 * ページサイズ で計算
		var pageSize, pageCount int64
		_ = db.QueryRow("PRAGMA page_size").Scan(&pageSize)
		_ = db.QueryRow("PRAGMA page_count").Scan(&pageCount)
		dbSizeMB := float64(pageSize*pageCount) / 1024 / 1024

		// 3. (オプション) 本日の活動状況
		var logsToday int
		_ = db.QueryRow("SELECT COUNT(*) FROM logs WHERE created_at > date('now', 'localtime')").Scan(&logsToday)

		return c.JSON(fiber.Map{
			"database": fiber.Map{
				"total_sessions": totalSessions,
				"total_logs":     totalLogs,
				"logs_today":     logsToday,
				"size_mb":        fmt.Sprintf("%.2f MB", dbSizeMB),
			},
			"system": fiber.Map{
				"goroutines": runtime.NumGoroutine(),
				"latency_ms": fiber.Map{
					"p50":     fmt.Sprintf("%.2f", stats.GetPercentile(50)),
					"p95":     fmt.Sprintf("%.2f", stats.GetPercentile(95)),
					"p99":     fmt.Sprintf("%.2f", stats.GetPercentile(99)),
					"samples": stats.SampleCount(), // 現在の蓄積数
				},
				// 他の動的な指標があればここに追加
			},
		})
	}
}
