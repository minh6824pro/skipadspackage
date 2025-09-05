package handler

import (
	"SkipAds/internal"
	"SkipAds/internal/models"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type Controller struct {
	service internal.Service
}

func NewController(service internal.Service) *Controller {
	return &Controller{service: service}
}

func (controller *Controller) CreatePurchase(ctx *gin.Context) {
	var m models.Purchase
	if err := ctx.ShouldBindJSON(&m); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := controller.service.CreatePurchase(ctx, &m)

	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, m)
}

func (controller *Controller) CreateBatchPurchase(ctx *gin.Context) {
	var m []models.BatchPurchaseRequest
	if err := ctx.ShouldBindJSON(&m); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := controller.service.CreateBatchPurchase(ctx, 2, m)

	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, m)
}

func (controller *Controller) GetPurchaseByUserID(ctx *gin.Context) {
	id, _ := strconv.Atoi(ctx.Param("id"))
	purchase, err := controller.service.GetPurchaseByUserID(ctx, uint(id))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, purchase)
}

func (controller *Controller) GetRemainingSkipAds(ctx *gin.Context) {
	id, _ := strconv.Atoi(ctx.Param("id"))
	skipAds, err := controller.service.GetRemainingSkipAds2(ctx, uint(id))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, skipAds)
}

func (controller *Controller) SkipAds(ctx *gin.Context) {
	var user models.User
	if err := ctx.ShouldBindJSON(&user); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := controller.service.SkipAds2(ctx, user.ID, 1)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true})
}
