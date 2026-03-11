package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Address                         string
	StorageDriver                   string
	SQLitePath                      string
	AdminEmail                      string
	AdminPassword                   string
	AdminToken                      string
	RegistrationToken               string
	DNSDomain                       string
	RelayAddresses                  []string
	ExitNodeID                      string
	ExitNodeMode                    string
	ExitNodeAllowLAN                bool
	ExitNodeAllowInternet           bool
	ExitNodeDNSMode                 string
	NodeOnlineWindow                time.Duration
	EndpointFreshnessWindow         time.Duration
	TransportFreshnessWindow        time.Duration
	DirectAttemptCooldown           time.Duration
	DirectAttemptLead               time.Duration
	DirectAttemptWindow             time.Duration
	DirectAttemptBurstInterval      time.Duration
	DirectAttemptRetention          time.Duration
	DirectAttemptManualRecoverAfter time.Duration
}

func Load() Config {
	return Config{
		Address:                         getEnv("CONTROLPLANE_ADDRESS", ":8080"),
		StorageDriver:                   getEnv("CONTROLPLANE_STORAGE_DRIVER", "sqlite"),
		SQLitePath:                      getEnv("CONTROLPLANE_SQLITE_PATH", "data/controlplane.db"),
		AdminEmail:                      getEnv("CONTROLPLANE_ADMIN_EMAIL", "admin@example.com"),
		AdminPassword:                   getEnv("CONTROLPLANE_ADMIN_PASSWORD", "dev-password"),
		AdminToken:                      getEnv("CONTROLPLANE_ADMIN_TOKEN", "dev-admin-token"),
		RegistrationToken:               getEnv("CONTROLPLANE_REGISTRATION_TOKEN", "dev-register-token"),
		DNSDomain:                       getEnv("CONTROLPLANE_DNS_DOMAIN", "internal.net"),
		RelayAddresses:                  splitCSV(getEnv("CONTROLPLANE_RELAYS", "relay-ap-1.example.net:3478,relay-us-1.example.net:3478")),
		ExitNodeID:                      getEnv("CONTROLPLANE_EXIT_NODE_ID", ""),
		ExitNodeMode:                    getEnv("CONTROLPLANE_EXIT_NODE_MODE", "enforced"),
		ExitNodeAllowLAN:                getEnvBool("CONTROLPLANE_EXIT_NODE_ALLOW_LAN", true),
		ExitNodeAllowInternet:           getEnvBool("CONTROLPLANE_EXIT_NODE_ALLOW_INTERNET", true),
		ExitNodeDNSMode:                 getEnv("CONTROLPLANE_EXIT_NODE_DNS_MODE", "follow_exit"),
		NodeOnlineWindow:                getEnvDuration("CONTROLPLANE_NODE_ONLINE_WINDOW", 30*time.Second),
		EndpointFreshnessWindow:         getEnvDuration("CONTROLPLANE_ENDPOINT_FRESHNESS_WINDOW", 45*time.Second),
		TransportFreshnessWindow:        getEnvDuration("CONTROLPLANE_TRANSPORT_FRESHNESS_WINDOW", 30*time.Second),
		DirectAttemptCooldown:           getEnvDuration("CONTROLPLANE_DIRECT_ATTEMPT_COOLDOWN", 10*time.Second),
		DirectAttemptLead:               getEnvDuration("CONTROLPLANE_DIRECT_ATTEMPT_LEAD", 150*time.Millisecond),
		DirectAttemptWindow:             getEnvDuration("CONTROLPLANE_DIRECT_ATTEMPT_WINDOW", 600*time.Millisecond),
		DirectAttemptBurstInterval:      getEnvDuration("CONTROLPLANE_DIRECT_ATTEMPT_BURST_INTERVAL", 80*time.Millisecond),
		DirectAttemptRetention:          getEnvDuration("CONTROLPLANE_DIRECT_ATTEMPT_RETENTION", 2*time.Second),
		DirectAttemptManualRecoverAfter: getEnvDuration("CONTROLPLANE_DIRECT_ATTEMPT_MANUAL_RECOVER_AFTER", 30*time.Second),
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

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		value = strings.TrimSpace(value)
		if value == "" {
			return fallback
		}
		duration, err := time.ParseDuration(value)
		if err == nil {
			return duration
		}
	}
	return fallback
}
