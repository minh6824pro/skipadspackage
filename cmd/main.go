package main

import (
	"SkipAds/api/handler"
	"SkipAds/internal"
	"SkipAds/internal/config"
	"SkipAds/internal/database"
	"github.com/gin-gonic/gin"
)

func main() {

	database.ConnectDatabase()
	database.AutoMigrate()

	config.InitRedis()
	//db := database.DB
	redisService := internal.NewRedisPackageService(config.RedisClient)

	r := gin.Default()

	api := r.Group("/api")
	handler.RegisterRoutes(api, database.DB, redisService)
	r.Run(":8080")

}
