package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/swagger"

	"ranklogger/config"
	"ranklogger/database"
	"ranklogger/handlers"
	"ranklogger/middleware"
)

func main() {
	// 1. 設定ファイルの読み込み
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Load config failed: %v", err)
	}

	// 2. データベースの初期化
	// InitDBの中で、テーブル作成や仮想列の追加が実行されます
	db, err := database.InitDB(cfg)
	if err != nil {
		log.Fatalf("Initialize DB failed: %v", err)
	}

	// 3. Fiberアプリのインスタンス生成
	app := fiber.New(fiber.Config{
		AppName:     "RankLogger API v1",
		JSONEncoder: sonic.Marshal,
		JSONDecoder: sonic.Unmarshal,
		ReadTimeout: time.Duration(cfg.Server.ReadTimeout) * time.Second,
	})

	// 4. ミドルウェアの設定

	// レイテンシ計測用statsとミドルウェア
	latencyStats := middleware.NewLatencyStats(1000)
	app.Use(middleware.NewLatencyMiddleware(latencyStats))

	// ユーザ単位のリミッター
	app.Use(middleware.UserLimit(cfg))

	// helmet: いろんなセキュリティ関連の設定をしてくれる
	app.Use(helmet.New())

	// logger: アクセスログをコンソールに表示
	app.Use(logger.New(logger.Config{
		CustomTags: map[string]logger.LogFunc{
			"real_ip": func(output logger.Buffer, c *fiber.Ctx, data *logger.Data, extraParam string) (int, error) {
				return output.WriteString(middleware.GetTrustedIP(c, cfg))
			},
		},
		Format: "${time} | ${status} | ${latency} | ${real_ip} | ${method} | ${path} | ${error}\n",
	}))

	// audit_logger: アクセスログをファイルに書き込み
	file, err := os.OpenFile("./data/system.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer file.Close()

	app.Use(logger.New(logger.Config{
		Output: file,
		CustomTags: map[string]logger.LogFunc{
			"full_ip": func(output logger.Buffer, c *fiber.Ctx, data *logger.Data, extraParam string) (int, error) {
				var full_ip = strings.Join(c.GetReqHeaders()["X-Forwarded-For"], ", ")
				if full_ip == "" {
					full_ip = middleware.GetTrustedIP(c, cfg)
				}
				return output.WriteString(full_ip)
			},
		},
		Format: "${time} | ${status} | ${latency} | ${full_ip} | ${method} | ${path} | ${error}\n",
	}))

	// recover: パニック（重大なエラー）が起きてもサーバーを落とさない
	app.Use(recover.New())

	// cors: 設定ファイルに基づいて外部からのアクセスを許可
	app.Use(cors.New(cors.Config{
		AllowOrigins: strings.Join(cfg.Server.CORSAllowOrigins, ", "),
	}))

	// 5. ルーティング（エンドポイントと処理の紐付け）
	app.Get("/doc/*", swagger.New(swagger.Config{
		URL: "/openapi.yaml", // ブラウザから見たyamlのパス
	}))

	// yamlファイル自体を静的ファイルとして公開しておく必要があります
	app.Static("/openapi.yaml", "./openapi.yaml")

	api := app.Group(cfg.Server.APIRootPath) // バージョニングをする

	// ランキング
	api.Get("/records/detail/:Tag?", middleware.GlobalLimit(cfg, "get_records_detail"),
		middleware.AdminAuth(cfg), handlers.GetRecords(db, cfg, true))
	api.Get("/records/:Tag?", middleware.GlobalLimit(cfg, "get_records"),
		handlers.GetRecords(db, cfg, false))
	api.Post("/records/:Tag?", middleware.GlobalLimit(cfg, "post_records"),
		middleware.GameClientAuth(cfg), handlers.PostRecord(db, cfg))
	api.Patch("/records/:SessionId", middleware.GlobalLimit(cfg, "patch_records"),
		middleware.AdminAuth(cfg), handlers.DisableRecord(db))
	api.Get("/ranks/:SessionId", middleware.GlobalLimit(cfg, "get_ranks"),
		middleware.GameClientAuth(cfg), handlers.GetRanks(db, cfg))

	// ログ
	api.Get("/logs/:SessionId", middleware.GlobalLimit(cfg, "get_logs"),
		middleware.AdminAuth(cfg), handlers.GetLogs(db))
	api.Post("/logs", middleware.GlobalLimit(cfg, "post_logs"),
		middleware.GameClientAuth(cfg), handlers.PostLogs(db, cfg))

	// メトリクス
	api.Get("/metrics", middleware.GlobalLimit(cfg, "get_metrics"),
		middleware.AdminAuth(cfg), handlers.GetMetrics(db, latencyStats))

	// 6. サーバーの起動
	go func() {
		log.Printf("Starting server (Port: %d)...", cfg.Server.Port)
		if err := app.Listen(fmt.Sprintf(":%d", cfg.Server.Port)); err != nil {
			log.Fatalf("failed to start server: %v", err)
		}
	}()

	// 7. 終了シグナルを待機するためのチャネルを作成
	// SIGINT (Ctrl+C) と SIGTERM (システムの停止指示) を監視
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// ここでシグナルが来るまでブロック（停止）
	<-quit
	log.Println("Gracefully shutting down...")

	// 8. 終了処理の期限（タイムアウト）を設定
	// 全てのリクエストが10秒以内に終わらなければ強制終了
	shutdownTimeout := 10 * time.Second

	if err := app.ShutdownWithTimeout(shutdownTimeout); err != nil {
		log.Printf("error during shutdown: %v", err)
	}

	// 9. データベース接続を閉じる
	if err := db.Close(); err != nil {
		log.Printf("error closing database: %v", err)
	}

	log.Println("Server stopped safely.")
}
