package main

import (
	"SkipAds/api/handler"
	"SkipAds/internal/database"
	"github.com/gin-gonic/gin"
)

func main() {

	database.ConnectDatabase()
	database.AutoMigrate()

	//db := database.DB

	r := gin.Default()

	api := r.Group("/api")
	handler.RegisterRoutes(api, database.DB)
	r.Run(":8080")

}
