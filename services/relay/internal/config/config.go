package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	ListenAddress string
	MappingTTL    time.Duration
}

func Load() Config {
	return Config{
		ListenAddress: getEnv("RELAY_ADDRESS", ":3478"),
		MappingTTL:    parseDuration(getEnv("RELAY_MAPPING_TTL", "2m"), 2*time.Minute),
	}
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func parseDuration(raw string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
