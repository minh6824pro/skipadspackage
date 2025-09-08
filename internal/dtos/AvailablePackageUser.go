package dtos

type AvailablePackageUser struct {
	PurchaseID     uint   `json:"purchase_id"`
	Remaining      uint32 `json:"remaining"`
	MaxUsagePerDay uint32 `json:"max_usage_per_day"`
	UsedToday      uint32 `json:"used_today"`
	ExpiresAtUnix  int64  `json:"expires_at_unix"`
}
