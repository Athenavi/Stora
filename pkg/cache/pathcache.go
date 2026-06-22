// Package cache provides caching utilities backed by Redis.
// When Redis is unavailable, operations silently degrade to no-ops.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// PathCache provides a Redis-backed cache for folder path lookups.
// Key format: "pathcache:{userID}:{path}" → folderID (JSON int64)
// Ponytail: single-flight TTL. Upgrade to LRU with full eviction if
// miss rate exceeds 20%.
type PathCache struct {
	client *redis.Client
	ttl    time.Duration
	prefix string
}

// NewPathCache creates a new Redis path cache.
// If client is nil, all operations are no-ops (graceful degradation).
func NewPathCache(client *redis.Client, ttl time.Duration) *PathCache {
	prefix := "pathcache"
	return &PathCache{client: client, ttl: ttl, prefix: prefix}
}

// key builds the Redis key.
func (c *PathCache) key(userID int64, path string) string {
	return fmt.Sprintf("%s:%d:%s", c.prefix, userID, path)
}

// Get retrieves a cached folder ID.
func (c *PathCache) Get(userID int64, path string) (int64, bool) {
	if c.client == nil {
		return 0, false
	}
	val, err := c.client.Get(context.Background(), c.key(userID, path)).Bytes()
	if err != nil {
		return 0, false
	}
	var folderID int64
	if err := json.Unmarshal(val, &folderID); err != nil {
		return 0, false
	}
	return folderID, true
}

// Set stores a folder ID in the cache.
func (c *PathCache) Set(userID int64, path string, folderID int64) {
	if c.client == nil {
		return
	}
	data, _ := json.Marshal(folderID)
	c.client.Set(context.Background(), c.key(userID, path), data, c.ttl)
}

// InvalidateUser removes all cache entries for a given user (prefix scan + delete).
func (c *PathCache) InvalidateUser(userID int64) {
	if c.client == nil {
		return
	}
	ctx := context.Background()
	pattern := fmt.Sprintf("%s:%d:*", c.prefix, userID)
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		c.client.Del(ctx, iter.Val())
	}
}
