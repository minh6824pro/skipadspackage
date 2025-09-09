package models

import "time"

type Purchase struct {
	ID        uint      `gorm:"primary_key;AUTO_INCREMENT" json:"id"`
	PackageID uint      `gorm:"package_id" json:"package_id"`
	UserID    uint32    `gorm:"user_id" json:"user_id"`
	Remaining uint32    `gorm:"remaining" json:"remaining"`
	CreatedAt time.Time `gorm:"created_at" json:"created_at"`
	ExpiresAt time.Time `gorm:"expires_at" json:"expires_at"`

	Usages  []Usage `gorm:"foreignKey:PurchaseID" json:"usage,omitempty"`
	Package Package `gorm:"foreignKey:PackageID" json:"package,omitempty"`
}
