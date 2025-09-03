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
	)

	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}
	log.Println("Migration successfully")
}
