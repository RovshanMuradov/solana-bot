// internal/storage/models/base.go
package models

import "time"

// BaseModel заменяет gorm.Model для большего контроля
type BaseModel struct {
	ID        uint       `gorm:"primarykey"`
	CreatedAt time.Time  `gorm:"default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time  `gorm:"default:CURRENT_TIMESTAMP"`
	DeletedAt *time.Time `gorm:"index"`
}
