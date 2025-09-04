package internal

import (
	"SkipAds/internal/models"
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"log"
	"time"
)

type Service interface {
	GetPurchaseByUserID(ctx context.Context, userID uint) ([]models.Purchase, error)
	CreatePurchase(ctx context.Context, purchase *models.Purchase) error
	GetRemainingSkipAds(ctx context.Context, userID uint) (uint32, error)
	SkipAds(ctx context.Context, userID uint) error
	GetRemainingSkipAds2(ctx context.Context, userID uint) (uint32, error)
	SkipAds2(ctx context.Context, userID uint, quantity uint32) error
}

type serviceImpl struct {
	repo Repository
	db   *gorm.DB
}

func NewService(repo Repository, db *gorm.DB) Service {
	return &serviceImpl{repo: repo, db: db}
}

func (s serviceImpl) GetRemainingSkipAds(ctx context.Context, userID uint) (uint32, error) {
	purchases, err := s.repo.ListNotExpirePurchaseByUserID(ctx, userID)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	remaining := uint32(0)
	for _, purchase := range purchases {
		temp := purchase.Package.Quantity
		usages := purchase.Usages

		// Check remaining skip ads of purchase
		for _, usage := range usages {
			temp = temp - usage.UseQuantity
		}
		// Check if  skip ads of purchase remaining < max usage per day
		if temp <= purchase.Package.MaxUsagePerDay {
			remaining += temp
		} else {
			// Get remaining usage of the day
			temp = purchase.Package.MaxUsagePerDay
			for _, usage := range usages {
				if (usage.CreatedAt.After(startOfDay) || usage.CreatedAt.Equal(startOfDay)) && temp > 0 {
					temp--
				} else {
					// usage sort by created_at desc, if not after/equal -> break
					break
				}
			}
			remaining += temp
		}
	}
	return remaining, nil
}

func (s serviceImpl) CreatePurchase(ctx context.Context, purchase *models.Purchase) error {
	pack, err := s.repo.GetPackage(ctx, purchase.PackageID)
	if err != nil {
		return err
	}
	purchase.ExpiresAt = time.Now().Add(time.Duration(pack.ExpiresAfter) * 24 * time.Hour)
	purchase.Remaining = pack.Quantity
	return s.repo.CreatePurchase(ctx, purchase)
}

func (s serviceImpl) GetPurchaseByUserID(ctx context.Context, userID uint) ([]models.Purchase, error) {
	return s.repo.ListPurchaseByUserID(ctx, userID)
}
func (s serviceImpl) SkipAds(ctx context.Context, userID uint) error {
	// Begin transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Use SELECT ... FOR UPDATE to lock the row
	var purchase models.Purchase
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND remaining > ?", userID, 0).
		First(&purchase).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			tx.Rollback()
			return errors.New("No skip ads left")
		}
		tx.Rollback()
		return err
	}

	// Insert skip ads usage for tracking
	usage := models.Usage{
		PurchaseID:  purchase.ID,
		UseQuantity: 1,
		UseFor:      "Skip Ads",
	}
	if err := tx.Save(&usage).Error; err != nil {
		tx.Rollback()

		return err
	}

	// Commit
	return tx.Commit().Error
}

func (s *serviceImpl) GetRemainingSkipAds2(ctx context.Context, userID uint) (uint32, error) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var results []struct {
		Remaining      uint32
		MaxUsagePerDay uint32
		UsedToday      uint32
	}

	err := s.db.Debug().Raw(`
        SELECT 
            p.remaining,
            pkg.max_usage_per_day,
            COALESCE(daily.used_today, 0) as used_today
        FROM purchases p
        JOIN packages pkg ON p.package_id = pkg.id
        LEFT JOIN package_daily_statuses daily ON p.id = daily.purchase_id AND daily.date = ?
        WHERE p.user_id = ? 
        AND p.expires_at > NOW() 
        AND p.remaining > 0
    `, today, userID).Scan(&results).Error

	log.Println(results)
	if err != nil {
		return 0, err
	}

	totalRemaining := uint32(0)
	for _, result := range results {
		availableToday := min(result.Remaining, result.MaxUsagePerDay-result.UsedToday)
		totalRemaining += availableToday
	}

	return totalRemaining, nil
}

func (s *serviceImpl) SkipAds2(ctx context.Context, userID uint, quantity uint32) error {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Get available packages with SELECT ... FOR UPDATE to lock row
	type AvailablePackageUser struct {
		PurchaseID     uint
		Remaining      uint32
		MaxUsagePerDay uint32
		UsedToday      uint32
		ExpiresAt      time.Time
	}

	// Get info of AvailablePackage
	var availablePackagesUser []AvailablePackageUser
	err := tx.Raw(`
        SELECT 
            p.id as purchase_id,
            p.remaining,
            pkg.max_usage_per_day,
            COALESCE(daily.used_today, 0) as used_today,
            p.expires_at
        FROM purchases p
        JOIN packages pkg ON p.package_id = pkg.id
        LEFT JOIN package_daily_statuses daily ON p.id = daily.purchase_id AND daily.date = ?
        WHERE p.user_id = ? 
        AND p.expires_at > NOW() 
        AND p.remaining > 0
        FOR UPDATE  
    `, today, userID).Scan(&availablePackagesUser).Error

	if err != nil {
		tx.Rollback()
		return err
	}

	// Calculate usable skip ads of today
	totalAvailable := uint32(0)
	for _, pkg := range availablePackagesUser {
		dailyAvailable := min(pkg.Remaining, pkg.MaxUsagePerDay-pkg.UsedToday)
		totalAvailable += dailyAvailable
	}
	if totalAvailable < quantity {
		tx.Rollback()
		return errors.New("insufficient quota")
	}

	// Distribute usage to packages
	remainingToUse := quantity

	for _, pkg := range availablePackagesUser {
		if remainingToUse == 0 {
			break
		}

		// Calculate usable skip ads of today
		dailyAvailable := min(pkg.Remaining, pkg.MaxUsagePerDay-pkg.UsedToday)
		useFromThis := min(remainingToUse, dailyAvailable)

		if useFromThis > 0 {
			// Update Purchase.remaining (skip ads ticket)
			result := tx.Model(&models.Purchase{}).
				Where("id = ? AND remaining >= ?", pkg.PurchaseID, useFromThis).
				Update("remaining", gorm.Expr("remaining - ?", useFromThis))

			if result.Error != nil {
				tx.Rollback()
				return result.Error
			}

			if result.RowsAffected == 0 {
				tx.Rollback()
				return errors.New("concurrent usage detected - insufficient quota")
			}

			// Update PackageDailyStatus.used_today
			err = s.updateOrCreateDailyStatus(ctx, tx, pkg.PurchaseID, today, useFromThis, pkg.MaxUsagePerDay)
			if err != nil {
				tx.Rollback()
				return err
			}

			// Create usage for tracking
			usage := models.Usage{
				PurchaseID:  pkg.PurchaseID,
				UseQuantity: useFromThis,
				UseFor:      "skip_ads",
				CreatedAt:   time.Now(),
			}

			err = tx.Create(&usage).Error
			if err != nil {
				tx.Rollback()
				return err
			}

			remainingToUse -= useFromThis
		}
	}

	if remainingToUse > 0 {
		tx.Rollback()
		return errors.New("failed to allocate all quota")
	}

	return tx.Commit().Error
}

func (s *serviceImpl) updateOrCreateDailyStatus(ctx context.Context, tx *gorm.DB, purchaseID uint, date time.Time, usedQuantity uint32, maxUsagePerDay uint32) error {
	var dailyStatus models.PackageDailyStatus
	err := tx.Where("purchase_id = ? AND date = ?", purchaseID, date).
		First(&dailyStatus).Error

	if err == gorm.ErrRecordNotFound {
		// Create package daily
		dailyStatus = models.PackageDailyStatus{
			PurchaseID:     purchaseID,
			Date:           date,
			UsedToday:      usedQuantity,
			MaxUsagePerDay: maxUsagePerDay,
		}
		return tx.Create(&dailyStatus).Error
	} else if err != nil {
		return err
	}

	// Update existing
	return tx.Model(&dailyStatus).
		Where("id = ?", dailyStatus.ID).
		Update("used_today", gorm.Expr("used_today + ?", usedQuantity)).Error
}

func min(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
