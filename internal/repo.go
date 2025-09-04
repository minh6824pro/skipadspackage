package internal

import (
	"SkipAds/internal/models"
	"context"
	"gorm.io/gorm"
	"time"
)

type Repository interface {
	CreatePurchase(ctx context.Context, purchase *models.Purchase) error
	ListPurchaseByUserID(ctx context.Context, userID uint) ([]models.Purchase, error)
	ListNotExpirePurchaseByUserID(ctx context.Context, userID uint) ([]models.Purchase, error)
	GetPackage(ctx context.Context, packageID uint) (*models.Package, error)
	//FindUsageByPurchaseID(ctx context.Context, purchaseID uint) (*models.Usage, error)
}

type repositoryImpl struct {
	DB *gorm.DB
}

func (r repositoryImpl) GetPackage(ctx context.Context, packageID uint) (*models.Package, error) {
	var pack models.Package
	if err := r.DB.Find(&pack, packageID).Error; err != nil {
		return nil, err
	}
	return &pack, nil
}

func NewRepositoryImpl(db *gorm.DB) Repository {
	return repositoryImpl{DB: db}
}

func (r repositoryImpl) CreatePurchase(ctx context.Context, purchase *models.Purchase) error {
	if err := r.DB.Create(purchase).Error; err != nil {
		return err
	}
	return nil
}

// ListPurchaseByUserID
func (r repositoryImpl) ListNotExpirePurchaseByUserID(ctx context.Context, userID uint) ([]models.Purchase, error) {
	var purchases []models.Purchase
	if err := r.DB.Where("user_id = ? AND expires_at> ? ", userID, time.Now()).
		Preload("Package").
		Preload("Usages", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at DESC")
		}).Find(&purchases).Error; err != nil {
		return nil, err
	}
	return purchases, nil
}

// ListRemainingPurchaseByUserID
func (r repositoryImpl) ListPurchaseByUserID(ctx context.Context, userID uint) ([]models.Purchase, error) {
	var purchases []models.Purchase
	err := r.DB.Where("user_id = ?", userID).
		Preload("Package").
		Find(&purchases).Error
	if err != nil {
		return nil, err
	}
	return purchases, nil
}
