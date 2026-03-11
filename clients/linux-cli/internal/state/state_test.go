package state

import (
	"path/filepath"
	"testing"

	"nodeweave/packages/contracts/go/api"
)

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")

	want := File{
		ServerURL: "http://127.0.0.1:8080",
		Device: api.Device{
			ID:   "dev_1",
			Name: "cli",
		},
		Node: api.Node{
			ID:        "node_1",
			OverlayIP: "100.64.0.10",
		},
		NodeToken: "node_token",
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("save state: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	if got.ServerURL != want.ServerURL || got.Device.ID != want.Device.ID || got.Node.ID != want.Node.ID || got.NodeToken != want.NodeToken {
		t.Fatalf("unexpected state roundtrip: %#v", got)
	}
}
