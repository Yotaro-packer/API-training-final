package models

// GameRecordInput は POST /records で受け取るデータの構造
type GameRecordInput struct {
	UUID      string                 `json:"uuid"`
	PlayCount int                    `json:"play_count"`
	Data      map[string]interface{} `json:"data"` // JSONの中身は動的なので map で受ける
}
