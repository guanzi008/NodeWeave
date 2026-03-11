package agent

import (
	"os"
	"path/filepath"
	"testing"

	"nodeweave/clients/linux-agent/internal/config"
	"nodeweave/clients/linux-agent/internal/state"
	"nodeweave/packages/contracts/go/api"
)

func TestLoadOrCreateIdentityReusesExistingKey(t *testing.T) {
	privateKeyPath := filepath.Join(t.TempDir(), "node.key")

	first, err := loadOrCreateIdentity(privateKeyPath)
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}
	if first.PrivateKey == "" || first.PublicKey == "" {
		t.Fatalf("expected generated key pair, got %#v", first)
	}

	info, err := os.Stat(privateKeyPath)
	if err != nil {
		t.Fatalf("stat private key: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("unexpected private key permissions %o", info.Mode().Perm())
	}

	second, err := loadOrCreateIdentity(privateKeyPath)
	if err != nil {
		t.Fatalf("reload identity: %v", err)
	}
	if second != first {
		t.Fatalf("expected identity roundtrip to match, got first=%#v second=%#v", first, second)
	}
}

func TestResolveIdentityAllowsPrivateKeyRotationAgainstEnrolledState(t *testing.T) {
	privateKeyPath := filepath.Join(t.TempDir(), "node.key")
	loaded, err := loadOrCreateIdentity(privateKeyPath)
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}

	svc := &Service{
		cfg: config.Config{
			PrivateKeyPath: privateKeyPath,
		},
		state: state.File{
			Node: api.Node{
				ID:        "node-1",
				PublicKey: loaded.PublicKey + "-different",
			},
			NodeToken: "token",
		},
	}

	resolved, err := svc.resolveIdentity()
	if err != nil {
		t.Fatalf("resolve identity after private key rotation: %v", err)
	}
	if resolved.PublicKey != loaded.PublicKey {
		t.Fatalf("expected rotated private key public key %q, got %q", loaded.PublicKey, resolved.PublicKey)
	}
}

func TestResolveIdentityFallsBackToStoredNodePublicKey(t *testing.T) {
	svc := &Service{
		cfg: config.Config{},
		state: state.File{
			Node: api.Node{
				ID:        "node-1",
				PublicKey: "stored-public-key",
			},
			NodeToken: "token",
		},
	}

	resolved, err := svc.resolveIdentity()
	if err != nil {
		t.Fatalf("resolve identity from stored node: %v", err)
	}
	if resolved.PublicKey != "stored-public-key" {
		t.Fatalf("expected stored public key, got %q", resolved.PublicKey)
	}
}
