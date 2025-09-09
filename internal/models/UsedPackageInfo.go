package models

type UsedPackageInfo struct {
	PurchaseID     uint   `json:"purchase_id"`
	TicketsUsed    uint32 `json:"tickets_used"`
	RemainingAfter uint32 `json:"remaining_after"`
	UsedTodayAfter uint32 `json:"used_today_after"`
	MaxUsagePerDay uint32 `json:"max_usage_per_day"`
}
