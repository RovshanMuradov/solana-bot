// internal/storage/models/transaction.go
package models

import "time"

type Transaction struct {
	BaseModel
	Signature     string     `gorm:"unique;not null;type:varchar(88)"`
	WalletAddress string     `gorm:"index;not null;type:varchar(44)"`
	TaskName      string     `gorm:"not null;type:varchar(100)"`
	DexName       string     `gorm:"not null;type:varchar(50)"`
	AmountIn      float64    `gorm:"type:decimal(20,9);not null"`
	AmountOut     float64    `gorm:"type:decimal(20,9)"`
	TokenIn       string     `gorm:"not null;type:varchar(44)"`
	TokenOut      string     `gorm:"not null;type:varchar(44)"`
	Status        string     `gorm:"not null;type:varchar(20)"`
	ErrorMessage  string     `gorm:"type:text"`
	PriorityFee   float64    `gorm:"type:decimal(20,9);not null"`
	ExecutionTime float64    `gorm:"type:decimal(10,3)"`
	BlockTime     *time.Time `gorm:"index"`
}
