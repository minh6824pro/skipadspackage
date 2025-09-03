package models

import "time"

type Purchase struct {
	ID        uint      `gorm:"primary_key;AUTO_INCREMENT" json:"id"`
	PackageID uint      `gorm:"package_id" json:"package_id"`
	UserID    uint      `gorm:"user_id;index:idx_user_remaining" json:"user_id"`
	Remaining uint      `gorm:"remaining;index:idx_user_remaining" json:"remaining"`
	CreatedAt time.Time `gorm:"created_at" json:"created_at"`
}
