package handler

import (
	"SkipAds/internal"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRoutes(router *gin.RouterGroup, db *gorm.DB, redisService internal.RedisPackageService) {
	repo := internal.NewRepositoryImpl(db)
	service := internal.NewService(repo, db, redisService)
	controller := NewController(service)

	ads := router.Group("")
	{
		ads.GET("/remaining_skip/:id", controller.GetRemainingSkipAds)
		ads.POST("/purchase/", controller.CreatePurchase)
		ads.GET("/purchase/:id", controller.GetPurchaseByUserID)
		ads.POST("/skip/", controller.SkipAds)
		ads.POST("/purchasebatch/", controller.CreateBatchPurchase)
		ads.GET("/remaining_skip", controller.GetRemainingSkipAdsAll)
	}
}
