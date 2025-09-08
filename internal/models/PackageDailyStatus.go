package models

import "time"

type PackageDailyStatus struct {
	ID             uint      `gorm:"primary_key" json:"id"`
	PurchaseID     uint      `gorm:"purchase_id;index:idx_daily_purchase_date" json:"purchase_id"`
	Date           time.Time `gorm:"date" json:"date"`
	UsedToday      uint32    `gorm:"used_today" json:"used_today"`
	MaxUsagePerDay uint32    `gorm:"max_usage_per_day" json:"max_usage_per_day"`
}
