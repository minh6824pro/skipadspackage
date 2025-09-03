package models

import "time"

type Usage struct {
	ID          uint      `gorm:"primary_key;AUTO_INCREMENT" json:"id"`
	PurchaseID  uint      `gorm:"package_id" json:"purchase_id"`
	UseQuantity uint      `gorm:"use_quantity" json:"use_quantity"`
	UseFor      string    `gorm:"use_for" json:"use_for"`
	CreatedAt   time.Time `gorm:"created_at" json:"created_at"`
}
