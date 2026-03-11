package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nodeweave/packages/runtime/go/secureudp"
)

type identity struct {
	PrivateKey string
	PublicKey  string
}

func (s *Service) resolveIdentity() (identity, error) {
	privateKeyPath := strings.TrimSpace(s.cfg.PrivateKeyPath)
	if privateKeyPath != "" {
		return loadOrCreateIdentity(privateKeyPath)
	}

	publicKey := strings.TrimSpace(s.cfg.PublicKey)
	if publicKey == "" {
		publicKey = strings.TrimSpace(s.state.Node.PublicKey)
	}
	if publicKey == "" {
		publicKey = fmt.Sprintf("devpub-%d", time.Now().UnixNano())
	}
	return identity{PublicKey: publicKey}, nil
}

func loadOrCreateIdentity(path string) (identity, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return identity{}, fmt.Errorf("private key path is required")
	}

	raw, err := os.ReadFile(path)
	if err == nil {
		privateKey := strings.TrimSpace(string(raw))
		publicKey, err := secureudp.PublicKeyFromPrivateHex(privateKey)
		if err != nil {
			return identity{}, fmt.Errorf("derive public key from %s: %w", path, err)
		}
		return identity{
			PrivateKey: privateKey,
			PublicKey:  publicKey,
		}, nil
	}
	if !os.IsNotExist(err) {
		return identity{}, fmt.Errorf("read private key %s: %w", path, err)
	}

	privateKey, publicKey, err := secureudp.GenerateKeyPair()
	if err != nil {
		return identity{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return identity{}, fmt.Errorf("create private key dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(privateKey), 0o600); err != nil {
		return identity{}, fmt.Errorf("write private key %s: %w", path, err)
	}
	return identity{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	}, nil
}
