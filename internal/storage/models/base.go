// internal/storage/models/base.go
package models

import "time"

// BaseModel заменяет gorm.Model для большего контроля
type BaseModel struct {
	ID        uint       `gorm:"primaryKey"`     // Рекомендуется использовать именно "primaryKey"
	CreatedAt time.Time  `gorm:"autoCreateTime"` // автоматическое заполнение времени создания
	UpdatedAt time.Time  `gorm:"autoUpdateTime"` // автоматическое обновление времени
	DeletedAt *time.Time `gorm:"index"`
}
