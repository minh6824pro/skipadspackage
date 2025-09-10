package database

import (
	"SkipAds/internal/models"
	"log"
)

func AutoMigrate() {
	err := DB.AutoMigrate(
		&models.Package{},
		&models.Purchase{},
		&models.Usage{},
		&models.User{},
		&models.PackageDailyStatus{},
	)

	DB.Exec("CREATE INDEX idx_purchases_user_expires_id ON purchases (user_id, expires_at, id)")
	DB.Exec("CREATE INDEX idx_daily_purchase_date ON package_daily_statuses (purchase_id, date)")

	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}
	log.Println("Migration successfully")
}
