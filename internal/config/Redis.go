package config

import (
	"context"
	"github.com/redis/go-redis/v9"
	"log"
	"strconv"
)

var (
	RedisClient *redis.Client
	RedisCtx    context.Context
)

func InitRedis() {
	addr := "localhost:6379"
	password := ""
	dbStr := "0"
	RedisCtx = context.Background()

	db, err := strconv.Atoi(dbStr)
	if err != nil {
		log.Printf("Invalid REDIS_DB value '%s', default to 0", dbStr)
		db = 0
	}

	RedisClient = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Kiểm tra kết nối Redis
	err = RedisClient.Ping(RedisCtx).Err()
	if err != nil {
		log.Printf("Failed to connect to Redis: %v", err)
	} else {
		log.Println("Connected to Redis successfully")
	}
}
