// internal/storage/models/task.go
package models

import "time"

type TaskHistory struct {
	BaseModel
	TaskName             string `gorm:"not null;type:varchar(100)"`
	Status               string `gorm:"not null;type:varchar(20)"`
	StartedAt            *time.Time
	CompletedAt          *time.Time
	SuccessCount         int     `gorm:"default:0"`
	ErrorCount           int     `gorm:"default:0"`
	TotalVolume          float64 `gorm:"type:decimal(20,9);default:0"`
	AverageExecutionTime float64 `gorm:"type:decimal(10,3)"`
}
