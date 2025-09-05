package models

type BatchPurchaseRequest struct {
	PackageID uint   `json:"package_id"`
	Quantity  uint32 `json:"quantity"`
}
