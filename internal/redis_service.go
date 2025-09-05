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
	KeyPattern = "user:%d:packages" // user:123:packages
	DefaultTTL = 12 * time.Hour     // Cache 12 hours
)

func NewRedisPackageService(client *redis.Client) RedisPackageService {
	return &redisPackageService{
		client: client,
		ttl:    DefaultTTL,
	}
}

// Set packages array for user
func (r *redisPackageService) SetUserPackages(ctx context.Context, userID uint, packages []models.AvailablePackageUser) error {
	key := fmt.Sprintf(KeyPattern, userID)

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
	key := fmt.Sprintf(KeyPattern, userID)

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
	key := fmt.Sprintf(KeyPattern, userID)
	return r.client.Del(ctx, key).Err()
}

func (r *redisPackageService) ExecuteLuaScript(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return r.client.Eval(ctx, script, keys, args...).Result()
}

func (r *redisPackageService) AcquireLock(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	luaScript := `
        if redis.call("GET", KEYS[1]) == false then
            redis.call("SET", KEYS[1], ARGV[1], "EX", ARGV[2])
            return 1
        else
            return 0
        end
    `

	result, err := r.client.Eval(ctx, luaScript, []string{key}, "locked", int(expiration.Seconds())).Result()
	if err != nil {
		return false, err
	}

	return result.(int64) == 1, nil
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
