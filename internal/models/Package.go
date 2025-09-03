package models

import "time"

type Package struct {
	ID        uint      `gorm:"primary_key" json:"id"`
	Name      string    `gorm:"name" json:"name"`
	Quantity  uint      `gorm:"quantity" json:"quantity"`
	CreatedAt time.Time `gorm:"created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"updated_at" json:"updated_at"`
}
