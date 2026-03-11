package config

import (
	"os"
	"strings"
)

type Config struct {
	Address               string
	StorageDriver         string
	SQLitePath            string
	AdminEmail            string
	AdminPassword         string
	AdminToken            string
	RegistrationToken     string
	DNSDomain             string
	RelayAddresses        []string
	ExitNodeID            string
	ExitNodeMode          string
	ExitNodeAllowLAN      bool
	ExitNodeAllowInternet bool
	ExitNodeDNSMode       string
}

func Load() Config {
	return Config{
		Address:               getEnv("CONTROLPLANE_ADDRESS", ":8080"),
		StorageDriver:         getEnv("CONTROLPLANE_STORAGE_DRIVER", "sqlite"),
		SQLitePath:            getEnv("CONTROLPLANE_SQLITE_PATH", "data/controlplane.db"),
		AdminEmail:            getEnv("CONTROLPLANE_ADMIN_EMAIL", "admin@example.com"),
		AdminPassword:         getEnv("CONTROLPLANE_ADMIN_PASSWORD", "dev-password"),
		AdminToken:            getEnv("CONTROLPLANE_ADMIN_TOKEN", "dev-admin-token"),
		RegistrationToken:     getEnv("CONTROLPLANE_REGISTRATION_TOKEN", "dev-register-token"),
		DNSDomain:             getEnv("CONTROLPLANE_DNS_DOMAIN", "internal.net"),
		RelayAddresses:        splitCSV(getEnv("CONTROLPLANE_RELAYS", "relay-ap-1.example.net:3478,relay-us-1.example.net:3478")),
		ExitNodeID:            getEnv("CONTROLPLANE_EXIT_NODE_ID", ""),
		ExitNodeMode:          getEnv("CONTROLPLANE_EXIT_NODE_MODE", "enforced"),
		ExitNodeAllowLAN:      getEnvBool("CONTROLPLANE_EXIT_NODE_ALLOW_LAN", true),
		ExitNodeAllowInternet: getEnvBool("CONTROLPLANE_EXIT_NODE_ALLOW_INTERNET", true),
		ExitNodeDNSMode:       getEnv("CONTROLPLANE_EXIT_NODE_DNS_MODE", "follow_exit"),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func getEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}
