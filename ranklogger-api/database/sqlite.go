package database

import (
	"database/sql"
	"fmt"
	"log"

	"ranklogger-api/config"

	_ "github.com/mattn/go-sqlite3" // SQLiteドライバー
)

func InitDB(cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", cfg.Server.DBPath)
	if err != nil {
		return nil, err
	}

	// 1. 基本となるテーブルの作成
	// SESSIONテーブル: JSONデータをそのまま入れる data カラムを持つ
	createSessionTable := `
	CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uuid TEXT NOT NULL,
		play_count INTEGER NOT NULL,
		data BLOB,
		disable BOOLEAN DEFAULT FALSE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(uuid, play_count)
	);`

	if _, err := db.Exec(createSessionTable); err != nil {
		return nil, err
	}

	// 2. カラム情報を確認して、必要なら追加する
	if err := setupDynamicColumns(db, cfg); err != nil {
		return nil, err
	}

	return db, nil
}

// カラム追加のロジックだけを分けた補助関数（内部で使う用）
func setupDynamicColumns(db *sql.DB, cfg *config.Config) error {
	// 1. 現在のテーブルにある列名をすべて取得する
	rows, err := db.Query("PRAGMA table_info(sessions);")
	if err != nil {
		return err
	}
	defer rows.Close()

	existingColumns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, dtype string
		var notnull, pk int
		var dflt_value interface{}

		// PRAGMA table_info は cid, name, type, notnull, dflt_value, pk を返す
		if err := rows.Scan(&cid, &name, &dtype, &notnull, &dflt_value, &pk); err != nil {
			return err
		}
		existingColumns[name] = true // 存在する列名を記録
	}

	// 2. 設定ファイルのスキーマをループし、存在しない列だけ追加する
	for _, field := range cfg.Schema {
		if field.IsIndex {
			if _, exists := existingColumns[field.Name]; !exists {
				// 列が存在しない場合のみ、仮想列を追加
				alterSQL := fmt.Sprintf(
					"ALTER TABLE sessions ADD COLUMN %s %s GENERATED ALWAYS AS (data ->> '$.%s') VIRTUAL;",
					field.Name, field.Type, field.Name,
				)
				if _, err := db.Exec(alterSQL); err != nil {
					log.Printf("Failed to add column %s: %v", field.Name, err)
					continue
				}
				log.Printf("Added virtual column: %s", field.Name)
			}

			// インデックス作成 (CREATE INDEX IF NOT EXISTS はSQLite側で重複を弾いてくれるのでそのままでOK)
			indexSQL := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s ON sessions(%s);", field.Name, field.Name)
			db.Exec(indexSQL)
		}
	}
	return nil
}
