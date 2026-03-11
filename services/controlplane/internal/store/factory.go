package store

import (
	"fmt"
	"strings"

	"nodeweave/services/controlplane/internal/config"
)

func Open(cfg config.Config) (Store, error) {
	switch strings.ToLower(cfg.StorageDriver) {
	case "", "sqlite":
		return NewSQLiteStore(cfg)
	case "memory":
		return NewMemoryStore(cfg), nil
	default:
		return nil, fmt.Errorf("%w: unsupported storage driver %q", ErrInvalid, cfg.StorageDriver)
	}
}
