package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address         string
	MySQLHost       string
	MySQLPort       string
	MySQLDatabase   string
	MySQLUser       string
	MySQLPassword   string
	RedisAddress    string
	RedisPassword   string
	RedisDB         int
	RedisTLS        bool
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	ImageUploadDir  string
	AllowedOrigins  []string
	CookieSecure    bool
}

// Load는 운영에 필요한 비밀값을 환경변수에서만 읽어 구성합니다.
func Load() (Config, error) {
	redisDB, err := intEnv("REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}
	redisTLS, err := boolEnv("REDIS_TLS", false)
	if err != nil {
		return Config{}, err
	}
	cookieSecure, err := boolEnv("COOKIE_SECURE", true)
	if err != nil {
		return Config{}, err
	}

	config := Config{
		Address:         ":" + envOrDefault("SPINE_PORT", "8080"),
		MySQLHost:       envOrDefault("MYSQL_HOST", "127.0.0.1"),
		MySQLPort:       envOrDefault("MYSQL_PORT", "3306"),
		MySQLDatabase:   strings.TrimSpace(os.Getenv("MYSQL_DATABASE")),
		MySQLUser:       strings.TrimSpace(os.Getenv("MYSQL_USER")),
		MySQLPassword:   os.Getenv("MYSQL_PASSWORD"),
		RedisAddress:    envOrDefault("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:   os.Getenv("REDIS_PASSWORD"),
		RedisDB:         redisDB,
		RedisTLS:        redisTLS,
		JWTSecret:       os.Getenv("JWT_SECRET"),
		AccessTokenTTL:  30 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		ImageUploadDir:  envOrDefault("IMAGE_UPLOAD_DIR", "./images"),
		AllowedOrigins:  splitList(envOrDefault("ALLOWED_ORIGINS", "http://localhost:5173")),
		CookieSecure:    cookieSecure,
	}

	missing := make([]string, 0, 4)
	for name, value := range map[string]string{
		"MYSQL_DATABASE": config.MySQLDatabase,
		"MYSQL_USER":     config.MySQLUser,
		"MYSQL_PASSWORD": config.MySQLPassword,
		"JWT_SECRET":     config.JWTSecret,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("필수 환경변수가 없습니다: %s", strings.Join(missing, ", "))
	}
	if len(config.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET은 32바이트 이상이어야 합니다")
	}
	return config, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func intEnv(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s 값이 정수가 아닙니다: %w", name, err)
	}
	return value, nil
}

func boolEnv(name string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s 값이 불리언이 아닙니다: %w", name, err)
	}
	return value, nil
}
