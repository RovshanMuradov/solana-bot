// internal/storage/models/pool.go
package models

import (
	"time"
)

type PoolInfo struct {
	BaseModel
	PoolID     string    `gorm:"unique;not null;type:varchar(44)"`
	DexName    string    `gorm:"not null;type:varchar(50)"`
	TokenA     string    `gorm:"index;not null;type:varchar(44)"`
	TokenB     string    `gorm:"index;not null;type:varchar(44)"`
	Liquidity  float64   `gorm:"type:decimal(20,9)"`
	Price      float64   `gorm:"type:decimal(20,9)"`
	LastUpdate time.Time `gorm:"index;not null"`
}
