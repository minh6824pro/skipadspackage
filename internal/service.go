package internal

import (
	"SkipAds/internal/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"log"
	"time"
)

type Service interface {
	GetPurchaseByUserID(ctx context.Context, userID uint) ([]models.Purchase, error)
	CreatePurchase(ctx context.Context, purchase *models.Purchase) error
	GetRemainingSkipAds2(ctx context.Context, userID uint) (uint32, error)
	SkipAds2(ctx context.Context, userID uint, quantity uint32) error
	CreateBatchPurchase(ctx context.Context, userID uint32, requests []models.BatchPurchaseRequest) error
}

type serviceImpl struct {
	repo         Repository
	redisService RedisPackageService
	db           *gorm.DB
}

func NewService(repo Repository, db *gorm.DB, redisService RedisPackageService) Service {
	return &serviceImpl{repo: repo, db: db, redisService: redisService}
}

func (s *serviceImpl) CreatePurchase(ctx context.Context, purchase *models.Purchase) error {
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

func (s *serviceImpl) GetRemainingSkipAds2(ctx context.Context, userID uint) (uint32, error) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var results []struct {
		Remaining      uint32
		MaxUsagePerDay uint32
		UsedToday      uint32
	}

	err := s.db.Raw(`
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

	// Try Redis first

	cacheKey := fmt.Sprintf("user:%d:packages", userID)
	lockKey := fmt.Sprintf("user:%d:packages:lock", userID)
	currentTimeUnix := now.Unix()

	luaScript := `
-- Lua script cho Redis
-- KEYS[1]: Key (e.g., user:{user_id}:packages)
-- ARGV[1]: Unix timestamp  
-- ARGV[2]: Quantity of ticket need to use
local list_key = KEYS[1]
local current_time = tonumber(ARGV[1])
local tickets_needed = tonumber(ARGV[2])
local tickets_remaining = tickets_needed

-- Get cached packages array (JSON string)
local cached_data = redis.call('GET', list_key)
if not cached_data then
    return {0, "cache_miss"}
end

-- Parse JSON array
local packages = cjson.decode(cached_data)
local updated_packages = {}
local used_packages_info = {}

-- Step 1: Xử lý việc sử dụng tickets
for i, pkg in ipairs(packages) do
    if tickets_remaining <= 0 then
        break
    end
    
    -- Check available ticket: remaining ticket > 0, not expired yet, not reach daily usage limit
    if pkg.remaining > 0 and 
       pkg.expires_at_unix > current_time and
       pkg.used_today < pkg.max_usage_per_day then
        
        -- Calculate skip ads can be use from this package (not exceed daily limit)
        local daily_remaining = pkg.max_usage_per_day - pkg.used_today
        local tickets_to_use = math.min(
            tickets_remaining, 
            pkg.remaining,
            daily_remaining
        )
        
        if tickets_to_use > 0 then
            -- Update in memory
            pkg.remaining = pkg.remaining - tickets_to_use
            pkg.used_today = pkg.used_today + tickets_to_use
            tickets_remaining = tickets_remaining - tickets_to_use
            
            -- Save used packages info với max_usage_per_day
            table.insert(used_packages_info, {
                purchase_id = pkg.purchase_id,
                tickets_used = tickets_to_use,
                remaining_after = pkg.remaining,
                used_today_after = pkg.used_today,
                max_usage_per_day = pkg.max_usage_per_day
            })
        end
    end
end

-- Check if use enough ticket
if tickets_remaining > 0 then
    return {0, "insufficient_quota", tickets_remaining}
end

-- Bước 2: Filter all packages for caching include packages haven't use in this operation
for i, pkg in ipairs(packages) do
    -- Insert to cache after 
    if pkg.remaining > 0 and 
       pkg.expires_at_unix > current_time and
       pkg.used_today < pkg.max_usage_per_day then
        table.insert(updated_packages, pkg)
    end
end

-- Update cache with packages have usable skip ads only
if #updated_packages > 0 then
    redis.call('SET', list_key, cjson.encode(updated_packages), 'EX', 43200)
else
    -- Delete cache if no available package left
    redis.call('DEL', list_key)
end

-- Return success
return {1, "success", tickets_needed, cjson.encode(used_packages_info)}`

	// Execute Lua script
	result, err := s.redisService.ExecuteLuaScript(ctx, luaScript, []string{cacheKey}, currentTimeUnix, quantity)

	if err != nil {
		log.Printf("Redis Lua script error: %v, falling back to DB", err)
		return s.skipAdsFromDB(ctx, userID, quantity, today)
	}

	// Parse Lua script result
	resultSlice := result.([]interface{})
	success := resultSlice[0].(int64) == 1
	message := resultSlice[1].(string)

	if !success {
		if message == "cache_miss" {
			log.Printf("Cache miss for user %d, falling back to DB with lock", userID)
			return s.skipAdsFromDBWithLock(ctx, userID, luaScript, quantity, today, lockKey)
		} else if message == "insufficient_quota" {
			if check, _ := s.redisService.GetUserStock(ctx, userID, quantity); check {
				return s.skipAdsFromDBWithLock(ctx, userID, luaScript, quantity, today, lockKey)
			} else {
				return errors.New("insufficient_quota2")
			}
		} else {
			log.Printf("Redis operation failed: %s, falling back to DB", message)
			return s.skipAdsFromDBWithLock(ctx, userID, luaScript, quantity, today, lockKey)
		}
	}

	//  Sync To DB immediately
	if len(resultSlice) > 3 {
		usedPackagesJSON := resultSlice[3].(string)
		err := s.syncRedisToDatabase(ctx, userID, today, usedPackagesJSON, quantity)
		if err != nil {
			// DB sync failed - rollback Redis cache
			// Invalidate cache to prevent inconsistency
			s.redisService.DeleteUserPackages(ctx, userID)
			log.Printf("DB sync failed for user %d: %v, rolling back cache", userID, err)
			return errors.New(err.Error())
		}
	}

	return nil
}
func (s *serviceImpl) skipAdsFromDBWithLock(ctx context.Context, userID uint, luaScript string, quantity uint32, today time.Time, lockKey string) error {
	// Try to acquire distributed lock
	lockAcquired, err := s.redisService.AcquireLock(ctx, lockKey, 10*time.Second)
	if err != nil {
		log.Printf("Failed to acquire lock for user %d: %v", userID, err)
		return err
	}

	if !lockAcquired {
		// Lock not acquired, wait and retry checking cache
		log.Printf("Failed to acquire lock for user %d: %v, waitandretrycache", userID)
		return s.waitAndRetryCache(ctx, userID, luaScript, quantity, today, lockKey)
	}

	// Lock acquired, proceed with DB operation
	defer func() {
		if err := s.redisService.ReleaseLock(ctx, lockKey); err != nil {
			log.Printf("Failed to release lock for user %d: %v", userID, err)
		}
	}()

	// Double-check cache before going to DB
	cacheKey := fmt.Sprintf("user:%d:packages", userID)
	currentTimeUnix := time.Now().Unix()

	// Try cache one more time
	result, err := s.redisService.ExecuteLuaScript(ctx, luaScript, []string{cacheKey}, currentTimeUnix, quantity)
	if err == nil {
		resultSlice := result.([]interface{})
		success := resultSlice[0].(int64) == 1
		message := resultSlice[1].(string)

		if success {
			// Cache hit after acquiring lock, process normally
			if len(resultSlice) > 3 {
				usedPackagesJSON := resultSlice[3].(string)
				err := s.syncRedisToDatabase(ctx, userID, today, usedPackagesJSON, quantity)
				if err != nil {
					s.redisService.DeleteUserPackages(ctx, userID)
					return errors.New("DB sync failure")
				}
			}
			return nil
		} else if message != "cache_miss" {
			// Handle other errors
			if message == "insufficient_quota" {
				if check, _ := s.redisService.GetUserStock(ctx, userID, quantity); !check {
					return errors.New("insufficient_quota")
				}
			}
		}
	}

	// Proceed with DB operation
	return s.skipAdsFromDB(ctx, userID, quantity, today)
}

func (s *serviceImpl) waitAndRetryCache(ctx context.Context, userID uint, luaScript string, quantity uint32, today time.Time, lockKey string) error {
	maxRetries := 10
	retryDelay := 100 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		// Wait a bit
		time.Sleep(retryDelay)

		// Check if lock is still held
		lockExists, err := s.redisService.LockExists(ctx, lockKey)
		if err != nil {
			log.Printf("Error checking lock existence: %v", err)
			continue
		}

		if !lockExists {
			// Lock released, try cache again
			cacheKey := fmt.Sprintf("user:%d:packages", userID)
			currentTimeUnix := time.Now().Unix()

			result, err := s.redisService.ExecuteLuaScript(ctx, luaScript, []string{cacheKey}, currentTimeUnix, quantity)
			if err == nil {
				resultSlice := result.([]interface{})
				success := resultSlice[0].(int64) == 1
				message := resultSlice[1].(string)

				if success {
					// Cache populated by the lock holder
					if len(resultSlice) > 3 {
						usedPackagesJSON := resultSlice[3].(string)
						err := s.syncRedisToDatabase(ctx, userID, today, usedPackagesJSON, quantity)
						if err != nil {
							s.redisService.DeleteUserPackages(ctx, userID)
							return errors.New("DB sync failure")
						}
					}
					return nil
				} else if message == "insufficient_quota" {
					if check, _ := s.redisService.GetUserStock(ctx, userID, quantity); !check {
						return errors.New("insufficient_quota")

					}
				}
				// If still cache miss, continue retrying
			}
		}

		// Exponential backoff
		retryDelay *= 2
		if retryDelay > 1*time.Second {
			retryDelay = 1 * time.Second
		}
	}

	// Max retries exceeded, fallback to direct DB access
	log.Println("Max retries exceeded for user ", userID)
	return errors.New("max retries exceeded")
	//return s.skipAdsFromDB(ctx, userID, quantity, today)
}

func (s *serviceImpl) skipAdsFromDB(ctx context.Context, userID uint, quantity uint32, today time.Time) error {
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// =========================
	// Step 1: Lock user's purchases + packages + daily status
	// =========================
	var availablePackagesUser []models.AvailablePackageUser

	if err := tx.Raw(`
		SELECT
			p.id as purchase_id,
			p.remaining,
    		CAST(UNIX_TIMESTAMP(p.expires_at) AS UNSIGNED) AS expires_at_unix, 
			COALESCE(daily.used_today, 0) as used_today,
			pkg.max_usage_per_day
		FROM purchases p
		INNER JOIN packages pkg ON p.package_id = pkg.id
		LEFT JOIN package_daily_statuses daily
			ON p.id = daily.purchase_id AND daily.date = ?
		WHERE p.user_id = ?
		  AND p.expires_at > NOW()
		  AND p.remaining > 0
		  AND COALESCE(daily.used_today, 0) < pkg.max_usage_per_day
		ORDER BY p.expires_at ASC, p.id ASC
		LIMIT 50 
		FOR UPDATE OF p, daily
	`, today, userID).Scan(&availablePackagesUser).Error; err != nil {
		tx.Rollback()
		return err
	}

	if len(availablePackagesUser) == 0 {
		tx.Rollback()
		return errors.New("no available packages")
	}

	// =========================
	// Step 2: Check total available
	// =========================
	totalAvailable := uint32(0)
	for _, pkg := range availablePackagesUser {
		dailyAvailable := min(pkg.Remaining, pkg.MaxUsagePerDay-pkg.UsedToday)
		totalAvailable += dailyAvailable
	}

	if totalAvailable < quantity {
		tx.Rollback()
		s.redisService.SetUserStock(ctx, userID, quantity, false)
		return errors.New("insufficient quota3")
	}

	// =========================
	// Step 3: Allocate usage
	// =========================
	remainingToUse := quantity
	for i := range availablePackagesUser {
		if remainingToUse == 0 {
			break
		}

		pkg := &availablePackagesUser[i]
		dailyAvailable := min(pkg.Remaining, pkg.MaxUsagePerDay-pkg.UsedToday)
		useFromThis := min(remainingToUse, dailyAvailable)
		if useFromThis == 0 {
			continue
		}

		// 1️⃣ Update purchase.remaining (reuse lock)
		pkg.Remaining -= useFromThis
		pkg.UsedToday += useFromThis
		if err := tx.Model(&models.Purchase{}).
			Where("id = ?", pkg.PurchaseID).
			Update("remaining", pkg.Remaining).Error; err != nil {
			tx.Rollback()
			return err
		}

		// 2️⃣ Update daily status safely (lock inside updateOrCreateDailyStatus)
		if err := s.updateOrCreateDailyStatus(ctx, tx, pkg.PurchaseID, today, useFromThis, pkg.MaxUsagePerDay); err != nil {
			tx.Rollback()
			return err
		}

		// 3️⃣ Create usage record
		usage := models.Usage{
			PurchaseID:  pkg.PurchaseID,
			UseQuantity: useFromThis,
			UseFor:      "skip_ads",
			CreatedAt:   time.Now(),
		}
		if err := tx.Create(&usage).Error; err != nil {
			tx.Rollback()
			return err
		}

		remainingToUse -= useFromThis
	}

	if remainingToUse > 0 {
		tx.Rollback()
		return errors.New("failed to allocate all quota")
	}

	// =========================
	// Step 4: Commit transaction
	// =========================
	if err := tx.Commit().Error; err != nil {
		return err
	}

	// =========================
	// Step 5: Update cache
	// =========================
	var validPackagesForCache []models.AvailablePackageUser
	for _, pkg := range availablePackagesUser {
		if pkg.Remaining > 0 && pkg.UsedToday < pkg.MaxUsagePerDay {
			validPackagesForCache = append(validPackagesForCache, models.AvailablePackageUser{
				PurchaseID:     pkg.PurchaseID,
				Remaining:      pkg.Remaining,
				MaxUsagePerDay: pkg.MaxUsagePerDay,
				UsedToday:      pkg.UsedToday,
				ExpiresAtUnix:  pkg.ExpiresAtUnix,
			})
		}
	}

	if len(validPackagesForCache) > 0 {
		if err := s.redisService.SetUserPackages(ctx, userID, validPackagesForCache); err != nil {
			log.Printf("Failed to update cache after DB operation for user %d: %v", userID, err)
		}
	} else {
		if err := s.redisService.DeleteUserPackages(ctx, userID); err != nil {
			log.Printf("Failed to delete cache for user %d: %v", userID, err)
		}
	}

	return nil
}
func (s *serviceImpl) syncRedisToDatabase(ctx context.Context, userID uint, today time.Time, usedPackagesJSON string, quantity uint32) error {
	var usedPackages []models.UsedPackageInfo
	if err := json.Unmarshal([]byte(usedPackagesJSON), &usedPackages); err != nil {
		return fmt.Errorf("failed to parse used packages JSON: %w", err)
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Step 1: Collect all purchase IDs and lock them in one query
	purchaseIDs := make([]uint, len(usedPackages))
	usedPackageMap := make(map[uint]models.UsedPackageInfo)

	for i, usedPkg := range usedPackages {
		purchaseIDs[i] = usedPkg.PurchaseID
		usedPackageMap[usedPkg.PurchaseID] = usedPkg
	}

	// Step 2: Lock all purchases at once and validate remaining amounts
	var purchases []models.Purchase
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id IN (?)", purchaseIDs).
		Order("expires_at ASC, id ASC"). // Same ordering as skipAdsFromDB to prevent deadlocks
		Find(&purchases).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to lock purchases: %w", err)
	}

	// Step 3: Validate that all purchases have sufficient remaining amount
	for _, purchase := range purchases {
		usedPkg := usedPackageMap[purchase.ID]
		if purchase.Remaining < usedPkg.TicketsUsed {
			tx.Rollback()
			return fmt.Errorf("insufficient remaining for purchase %d: has %d, needs %d",
				purchase.ID, purchase.Remaining, usedPkg.TicketsUsed)
		}
	}

	// Step 4: Batch update all purchases
	for _, purchase := range purchases {
		usedPkg := usedPackageMap[purchase.ID]

		// Update remaining
		if err := tx.Model(&models.Purchase{}).
			Where("id = ?", purchase.ID).
			Update("remaining", gorm.Expr("remaining - ?", usedPkg.TicketsUsed)).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update purchase %d: %w", purchase.ID, err)
		}

		// Update daily status
		if err := s.updateOrCreateDailyStatus(ctx, tx, usedPkg.PurchaseID, today, usedPkg.TicketsUsed, usedPkg.MaxUsagePerDay); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update daily status for purchase %d: %w", purchase.ID, err)
		}

		// Create usage record
		usage := models.Usage{
			PurchaseID:  usedPkg.PurchaseID,
			UseQuantity: usedPkg.TicketsUsed,
			UseFor:      "skip_ads",
			CreatedAt:   time.Now(),
		}

		if err := tx.Create(&usage).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create usage record for purchase %d: %w", purchase.ID, err)
		}
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

func (s *serviceImpl) CreateBatchPurchase(ctx context.Context, userID uint32, requests []models.BatchPurchaseRequest) error {
	if len(requests) == 0 {
		return errors.New("no purchase requests provided")
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Step 1: Collect all unique package IDs
	packageIDs := make([]uint, 0, len(requests))
	packageIDSet := make(map[uint]bool)

	for _, req := range requests {
		if req.Quantity == 0 {
			continue
		}
		if !packageIDSet[req.PackageID] {
			packageIDs = append(packageIDs, req.PackageID)
			packageIDSet[req.PackageID] = true
		}
	}

	if len(packageIDs) == 0 {
		tx.Rollback()
		return errors.New("no valid purchase requests")
	}

	// Step 2: Batch fetch all packages
	var packages []models.Package
	if err := tx.Where("id IN (?)", packageIDs).Find(&packages).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to fetch packages: %w", err)
	}

	// Create package map for quick lookup
	packageMap := make(map[uint]models.Package)
	for _, pkg := range packages {
		packageMap[pkg.ID] = pkg
	}

	// Step 3: Validate all packages exist
	for _, req := range requests {
		if req.Quantity == 0 {
			continue
		}
		if _, exists := packageMap[req.PackageID]; !exists {
			tx.Rollback()
			return fmt.Errorf("package with ID %d not found", req.PackageID)
		}
	}

	// Step 4: Create all purchases
	now := time.Now()
	var allPurchases []models.Purchase

	for _, req := range requests {
		if req.Quantity == 0 {
			continue
		}

		pkg := packageMap[req.PackageID]
		expiresAt := now.Add(time.Duration(pkg.ExpiresAfter) * 24 * time.Hour)

		// Create multiple purchases for this package
		for i := uint32(0); i < req.Quantity; i++ {
			purchase := models.Purchase{
				UserID:    userID,
				PackageID: req.PackageID,
				ExpiresAt: expiresAt,
				Remaining: pkg.Quantity,
				CreatedAt: now,
			}
			allPurchases = append(allPurchases, purchase)
		}
	}

	// Step 5: Batch insert all purchases
	if len(allPurchases) > 0 {
		if err := tx.CreateInBatches(allPurchases, 100).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create purchases: %w", err)
		}
	}

	// Step 6: Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully created %d purchases for user %d", len(allPurchases), userID)
	return nil
}
