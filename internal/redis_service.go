package internal

import (
	"SkipAds/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisPackageService interface {
	SetUserPackages(ctx context.Context, userID uint, packages []models.AvailablePackageUser) error
	GetUserPackages(ctx context.Context, userID uint) ([]models.AvailablePackageUser, error)
	SetUserStock(ctx context.Context, userID uint, quantity uint32, haveStock bool) error
	GetUserStock(ctx context.Context, userID uint, quantity uint32) (bool, error)
	DeleteUserPackages(ctx context.Context, userID uint) error
	UpdateUserPackages(ctx context.Context, userID uint, packages []models.AvailablePackageUser) error
	ExecuteLuaScript(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
	AcquireLock(ctx context.Context, key string, expiration time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key string) error
	LockExists(ctx context.Context, key string) (bool, error)
}

type redisPackageService struct {
	client *redis.Client
	ttl    time.Duration
}

const (
	UserPackageKeyPattern    = "user:%d:packages" // user:123:packages
	UserCheckStockKeyPattern = "user:%d:checkstock:%d:false"
	DefaultTTL               = 12 * time.Hour // Cache 12 hours
)

func NewRedisPackageService(client *redis.Client) RedisPackageService {
	return &redisPackageService{
		client: client,
		ttl:    DefaultTTL,
	}
}

// SetUserStock:  haveStock=false → create key, if haveStock=true → delete key
func (r *redisPackageService) SetUserStock(ctx context.Context, userID uint, quantity uint32, haveStock bool) error {

	key := fmt.Sprintf(UserCheckStockKeyPattern, userID, quantity)
	if !haveStock {
		return r.client.Set(ctx, key, 1, 0).Err() // TTL=0 = không hết hạn
	} else {
		return r.client.Del(ctx, key).Err()
	}
}

// GetUserStock: key exists → no stock in db stock=false, key not exists → còn stock=true
func (r *redisPackageService) GetUserStock(ctx context.Context, userID uint, quantity uint32) (bool, error) {
	key := fmt.Sprintf(UserCheckStockKeyPattern, userID, quantity)
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists == 0, nil
}

// Set packages array for user
func (r *redisPackageService) SetUserPackages(ctx context.Context, userID uint, packages []models.AvailablePackageUser) error {
	key := fmt.Sprintf(UserPackageKeyPattern, userID)

	// Convert array to JSON
	jsonData, err := json.Marshal(packages)
	if err != nil {
		return fmt.Errorf("failed to marshal packages: %w", err)
	}

	// Set with TTL
	return r.client.Set(ctx, key, jsonData, r.ttl).Err()
}

// Get packages array for user
func (r *redisPackageService) GetUserPackages(ctx context.Context, userID uint) ([]models.AvailablePackageUser, error) {
	key := fmt.Sprintf(UserPackageKeyPattern, userID)

	// Get JSON from Redis
	jsonData, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get packages: %w", err)
	}

	// Unmarshal JSON to array
	var packages []models.AvailablePackageUser
	if err := json.Unmarshal([]byte(jsonData), &packages); err != nil {
		return nil, fmt.Errorf("failed to unmarshal packages: %w", err)
	}

	return packages, nil
}

// Update packages array for user
func (r *redisPackageService) UpdateUserPackages(ctx context.Context, userID uint, packages []models.AvailablePackageUser) error {
	// Same as SetUserPackages - overwrite the entire array
	return r.SetUserPackages(ctx, userID, packages)
}

// Delete packages for user
func (r *redisPackageService) DeleteUserPackages(ctx context.Context, userID uint) error {
	key := fmt.Sprintf(UserPackageKeyPattern, userID)
	return r.client.Del(ctx, key).Err()
}

func (r *redisPackageService) ExecuteLuaScript(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return r.client.Eval(ctx, script, keys, args...).Result()
}
func (r *redisPackageService) AcquireLock(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	// SET NX EX trực tiếp
	ok, err := r.client.SetNX(ctx, key, "locked", expiration).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (r *redisPackageService) ReleaseLock(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *redisPackageService) LockExists(ctx context.Context, key string) (bool, error) {
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}
