package models

type GameRecordInput struct {
	UUID      string                 `json:"uuid" validate:"required,uuid4"`       // UUID形式か
	PlayCount int                    `json:"play_count" validate:"required,min=1"` // 1以上か
	Data      map[string]interface{} `json:"data" validate:"required"`
}
