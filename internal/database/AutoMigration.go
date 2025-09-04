package database

import (
	"SkipAds/internal/models"
	"log"
)

func AutoMigrate() {
	err := DB.AutoMigrate(
		&models.Package{},
		&models.Usage{},
		&models.Purchase{},
		&models.User{},
		&models.PackageDailyStatus{},
	)

	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}
	log.Println("Migration successfully")
}
