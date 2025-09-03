package internal

import (
	"SkipAds/internal/models"
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service interface {
	GetPurchaseByUserID(ctx context.Context, userID uint) ([]models.Purchase, error)
	CreatePurchase(ctx context.Context, purchase *models.Purchase) error
	GetRemainingSkipAds(ctx context.Context, userID uint) (uint, error)
	SkipAds(ctx context.Context, userID uint) error
}

type serviceImpl struct {
	repo Repository
	db   *gorm.DB
}

func (s serviceImpl) GetRemainingSkipAds(ctx context.Context, userID uint) (uint, error) {
	purchase, err := s.repo.ListRemainingPurchaseByUserID(ctx, userID)
	if err != nil {
		return 0, err
	}
	count := uint(0)
	for _, p := range purchase {
		count += p.Remaining
	}
	return count, nil
}

func (s serviceImpl) CreatePurchase(ctx context.Context, purchase *models.Purchase) error {
	pack, err := s.repo.GetPackage(ctx, purchase.PackageID)
	if err != nil {
		return err
	}
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

	// Reduce skip ads remaining in purchased package
	purchase.Remaining -= 1
	if err := tx.Save(&purchase).Error; err != nil {
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

func NewService(repo Repository, db *gorm.DB) Service {
	return &serviceImpl{repo: repo, db: db}
}
