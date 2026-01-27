package main

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"ranklogger-api/config"
	"ranklogger-api/database"
	"ranklogger-api/handlers"
)

func main() {
	// 1. 設定ファイルの読み込み
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("設定の読み込みに失敗しました: %v", err)
	}

	// 2. データベースの初期化
	// InitDBの中で、テーブル作成や仮想列の追加が実行されます
	db, err := database.InitDB(cfg)
	if err != nil {
		log.Fatalf("データベースの初期化に失敗しました: %v", err)
	}
	defer db.Close() // アプリ終了時に安全にDBを閉じる

	// 3. Fiberアプリのインスタンス生成
	app := fiber.New(fiber.Config{
		AppName: "RankLogger API v1",
	})

	// 4. ミドルウェアの設定
	// logger: アクセスログをコンソールに表示
	app.Use(logger.New())
	// recover: パニック（重大なエラー）が起きてもサーバーを落とさない
	app.Use(recover.New())
	// cors: 設定ファイルに基づいて外部からのアクセスを許可
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.Server.CORSAllowOrigins[0], // 簡易的に最初の設定を使用
	}))

	// 5. ルーティング（エンドポイントと処理の紐付け）
	api := app.Group("/api/v1") // バージョニング

	api.Get("/records", handlers.GetRecords(db, cfg))

	api.Post("/records", handlers.PostRecord(db, cfg))

	// 6. サーバーの起動
	log.Printf("サーバーを起動します (Port: %d)...", cfg.Server.Port)
	if err := app.Listen(fmt.Sprintf(":%d", cfg.Server.Port)); err != nil {
		log.Fatalf("サーバーの起動に失敗しました: %v", err)
	}
}
