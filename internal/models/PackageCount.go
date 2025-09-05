package models

type PackageCount struct {
	PackageID uint  `json:"package_id"`
	Count     int64 `json:"count"`
}
