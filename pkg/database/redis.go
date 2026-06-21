package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient holds the Redis client.
var RedisClient *redis.Client

// ConnectRedis initializes the Redis connection.
func ConnectRedis(addr, password string, db int) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	RedisClient = rdb
	log.Printf("[Redis] Connected to %s (db %d)", addr, db)
	return rdb, nil
}

// CloseRedis gracefully closes the Redis connection.
func CloseRedis() {
	if RedisClient != nil {
		RedisClient.Close()
		log.Println("[Redis] Connection closed")
	}
}
