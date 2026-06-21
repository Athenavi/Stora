package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration.
type Config struct {
	// Server
	ServerPort  int
	ServerHost  string
	SecretKey   string
	Debug       bool
	TimeZone    string
	SiteName    string
	Domain      string

	// Database
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	// Redis
	RedisHost     string
	RedisPort     int
	RedisPassword string
	RedisDB       int

	// JWT
	JWTExpiration        time.Duration
	JWTRefreshExpiration time.Duration

	// Storage
	StorageDriver string       // "local" or "s3"
	TempFolder    string

	// S3
	S3Endpoint  string
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string

	// Email
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string
}

// Load reads configuration from environment variables and .env file.
func Load() *Config {
	// Load .env files (non-fatal if missing)
	_ = godotenv.Load()

	cfg := &Config{
		ServerPort:           getEnvInt("PORT", 9421),
		ServerHost:           getEnv("HOST", "0.0.0.0"),
		SecretKey:            getEnv("SECRET_KEY", ""),
		Debug:                getEnv("DEBUG", "false") == "true",
		TimeZone:             getEnv("TIME_ZONE", "Asia/Shanghai"),
		SiteName:             getEnv("TITLE", "Stora"),
		Domain:               getEnv("DOMAIN", "/"),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnvInt("DB_PORT", 5432),
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "stora"),

		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnvInt("REDIS_PORT", 6379),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),

		JWTExpiration:        time.Duration(getEnvInt("JWT_EXPIRATION_DELTA", 7200)) * time.Second,
		JWTRefreshExpiration: time.Duration(getEnvInt("REFRESH_TOKEN_EXPIRATION_DELTA", 64800)) * time.Second,

		StorageDriver: getEnv("STORAGE_DRIVER", "local"),
		TempFolder:    getEnv("TEMP_FOLDER", "temp/upload"),

		S3Endpoint:  getEnv("S3_ENDPOINT", ""),
		S3Region:    getEnv("S3_REGION", "us-east-1"),
		S3Bucket:    getEnv("S3_BUCKET", ""),
		S3AccessKey: getEnv("S3_ACCESS_KEY", ""),
		S3SecretKey: getEnv("S3_SECRET_KEY", ""),

		SMTPHost:     getEnv("SMTP_HOST", ""),
		SMTPPort:     getEnvInt("SMTP_PORT", 587),
		SMTPUser:     getEnv("SMTP_USER", ""),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:     getEnv("SMTP_FROM", ""),
	}

	// Ensure domain ends with /
	if cfg.Domain != "" && cfg.Domain[len(cfg.Domain)-1] != '/' {
		cfg.Domain += "/"
	}

	return cfg
}

// PostgresDSN returns the PostgreSQL connection string.
func (c *Config) PostgresDSN() string {
	passwordPart := ""
	if c.DBPassword != "" {
		passwordPart = ":" + c.DBPassword
	}
	return fmt.Sprintf("postgres://%s%s@%s:%d/%s?sslmode=disable",
		c.DBUser, passwordPart, c.DBHost, c.DBPort, c.DBName)
}

// RedisAddr returns the Redis address.
func (c *Config) RedisAddr() string {
	return fmt.Sprintf("%s:%d", c.RedisHost, c.RedisPort)
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}
