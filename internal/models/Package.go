package models

import "time"

type Package struct {
	ID             uint      `gorm:"primary_key" json:"id"`
	Name           string    `gorm:"name" json:"name"`
	Quantity       uint32    `gorm:"quantity" json:"quantity"`
	MaxUsagePerDay uint32    `gorm:"max_usage_per_day" json:"max_usage_per_day"`
	ExpiresAfter   uint32    `gorm:"expires_after" json:"expires_after"`
	CreatedAt      time.Time `gorm:"created_at" json:"created_at,omitempty"`
	UpdatedAt      time.Time `gorm:"updated_at" json:"updated_at,omitempty"`
}
