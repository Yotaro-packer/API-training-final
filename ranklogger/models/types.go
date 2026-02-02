package models

type GameRecordInput struct {
	UUID      string                 `json:"uuid" validate:"required,uuid4"`       // UUID形式か
	PlayCount int                    `json:"play_count" validate:"required,min=1"` // 1以上か
	Data      map[string]interface{} `json:"data" validate:"required"`
}

type LogInput struct {
	Type    int    `json:"type" validate:"required,min=1"`        // 0は未定義とするならmin=1
	Content string `json:"content" validate:"required,max=10000"` // 1ログの最大長を制限
}

type PostLogsRequest struct {
	UUID      string     `json:"uuid" validate:"required,uuid4"`
	PlayCount int        `json:"play_count" validate:"required,min=1"`
	Logs      []LogInput `json:"logs" validate:"required,min=1,max=300,dive"` // 1件以上のログを必須にし、中身も検証
}

type DisableRecordRequest struct {
	Disable bool `json:"disable"` // true で除外、false で復帰
}
