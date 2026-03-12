package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/services/controlplane/internal/config"
	"nodeweave/services/controlplane/internal/store"
)

func TestLoginSuccess(t *testing.T) {
	handler, cfg, cleanup := newTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", jsonBody(t, api.LoginRequest{
		Email:    cfg.AdminEmail,
		Password: cfg.AdminPassword,
	}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp api.LoginResponse
	decodeResponse(t, rec, &resp)
	if resp.AccessToken != cfg.AdminToken {
		t.Fatalf("expected admin token %q, got %q", cfg.AdminToken, resp.AccessToken)
	}
}

func TestDeviceRegistrationLifecycle(t *testing.T) {
	handler, cfg, cleanup := newTestServer(t)
	defer cleanup()

	regReq := api.DeviceRegistrationRequest{
		DeviceName:        "edge-win-01",
		Platform:          "windows",
		Version:           "0.1.0",
		PublicKey:         "pubkey-1",
		RegistrationToken: cfg.RegistrationToken,
	}

	regRec := sendJSON(t, handler, http.MethodPost, "/api/v1/devices/register", "", regReq)
	if regRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", regRec.Code, regRec.Body.String())
	}

	var regResp api.DeviceRegistrationResponse
	decodeResponse(t, regRec, &regResp)
	if regResp.NodeToken == "" {
		t.Fatal("expected node token in registration response")
	}
	if regResp.Node.OverlayIP == "" {
		t.Fatal("expected overlay ip")
	}

	nodeListRec := sendJSON(t, handler, http.MethodGet, "/api/v1/nodes", cfg.AdminToken, nil)
	if nodeListRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from list nodes, got %d: %s", nodeListRec.Code, nodeListRec.Body.String())
	}

	routeReq := api.CreateRouteRequest{
		NetworkCIDR: "10.10.0.0/16",
		ViaNodeID:   regResp.Node.ID,
		Priority:    100,
	}
	routeRec := sendJSON(t, handler, http.MethodPost, "/api/v1/routes", cfg.AdminToken, routeReq)
	if routeRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 from create route, got %d: %s", routeRec.Code, routeRec.Body.String())
	}

	hbReq := api.HeartbeatRequest{
		Endpoints:   []string{"203.0.113.10:51820"},
		RelayRegion: "ap",
		Status:      "online",
	}
	hbRec := sendJSON(t, handler, http.MethodPost, "/api/v1/nodes/"+regResp.Node.ID+"/heartbeat", regResp.NodeToken, hbReq)
	if hbRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from heartbeat, got %d: %s", hbRec.Code, hbRec.Body.String())
	}

	var hbResp api.HeartbeatResponse
	decodeResponse(t, hbRec, &hbResp)
	if hbResp.Node.Status != "online" {
		t.Fatalf("expected node status online, got %q", hbResp.Node.Status)
	}

	bootstrapRec := sendJSON(t, handler, http.MethodGet, "/api/v1/nodes/"+regResp.Node.ID+"/bootstrap", regResp.NodeToken, nil)
	if bootstrapRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from bootstrap, got %d: %s", bootstrapRec.Code, bootstrapRec.Body.String())
	}

	var bootstrap api.BootstrapConfig
	decodeResponse(t, bootstrapRec, &bootstrap)
	if len(bootstrap.Routes) != 1 {
		t.Fatalf("expected 1 route in bootstrap, got %d", len(bootstrap.Routes))
	}
	if bootstrap.Version < 2 {
		t.Fatalf("expected bootstrap version to increment, got %d", bootstrap.Version)
	}
}

func TestHeartbeatUpdatesNodePublicKey(t *testing.T) {
	handler, cfg, cleanup := newTestServer(t)
	defer cleanup()

	regResp := registerNode(t, handler, cfg)

	hbReq := api.HeartbeatRequest{
		Endpoints:   []string{"203.0.113.10:51820"},
		RelayRegion: "ap",
		Status:      "online",
		PublicKey:   "pubkey-rotated",
	}
	hbRec := sendJSON(t, handler, http.MethodPost, "/api/v1/nodes/"+regResp.Node.ID+"/heartbeat", regResp.NodeToken, hbReq)
	if hbRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from heartbeat, got %d: %s", hbRec.Code, hbRec.Body.String())
	}

	var hbResp api.HeartbeatResponse
	decodeResponse(t, hbRec, &hbResp)
	if hbResp.Node.PublicKey != "pubkey-rotated" {
		t.Fatalf("expected rotated public key, got %q", hbResp.Node.PublicKey)
	}
	if hbResp.BootstrapVersion < 2 {
		t.Fatalf("expected bootstrap version to increment after key rotation, got %d", hbResp.BootstrapVersion)
	}

	bootstrapRec := sendJSON(t, handler, http.MethodGet, "/api/v1/nodes/"+regResp.Node.ID+"/bootstrap", regResp.NodeToken, nil)
	if bootstrapRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from bootstrap, got %d: %s", bootstrapRec.Code, bootstrapRec.Body.String())
	}

	var bootstrap api.BootstrapConfig
	decodeResponse(t, bootstrapRec, &bootstrap)
	if bootstrap.Node.PublicKey != "pubkey-rotated" {
		t.Fatalf("expected bootstrap node public key to be rotated, got %q", bootstrap.Node.PublicKey)
	}
}

func TestHeartbeatRoundTripsNATReportAndDirectAttempts(t *testing.T) {
	handler, cfg, cleanup := newTestServer(t)
	defer cleanup()

	first := registerNode(t, handler, cfg)
	second := registerNode(t, handler, cfg)

	firstHB := api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "198.51.100.10:51820", Source: "stun"},
		},
		NATReport: api.NATReport{
			GeneratedAt:              time.Now().UTC(),
			MappingBehavior:          "stable_port",
			SelectedReflexiveAddress: "198.51.100.10:51820",
			Reachable:                true,
			Samples: []api.NATSample{
				{Server: "stun-a", Status: "reachable", ReflexiveAddress: "198.51.100.10:51820"},
			},
		},
	}
	firstHBRec := sendJSON(t, handler, http.MethodPost, "/api/v1/nodes/"+first.Node.ID+"/heartbeat", first.NodeToken, firstHB)
	if firstHBRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from first heartbeat, got %d: %s", firstHBRec.Code, firstHBRec.Body.String())
	}

	secondHB := api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "203.0.113.10:51820", Source: "stun"},
		},
		NATReport: api.NATReport{
			GeneratedAt:              time.Now().UTC(),
			MappingBehavior:          "varying_port",
			SelectedReflexiveAddress: "203.0.113.10:51820",
			Reachable:                true,
			Samples: []api.NATSample{
				{Server: "stun-b", Status: "reachable", ReflexiveAddress: "203.0.113.10:51820"},
			},
		},
	}
	secondHBRec := sendJSON(t, handler, http.MethodPost, "/api/v1/nodes/"+second.Node.ID+"/heartbeat", second.NodeToken, secondHB)
	if secondHBRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from second heartbeat, got %d: %s", secondHBRec.Code, secondHBRec.Body.String())
	}

	var hbResp api.HeartbeatResponse
	decodeResponse(t, secondHBRec, &hbResp)
	if len(hbResp.DirectAttempts) == 0 {
		t.Fatalf("expected direct attempts in heartbeat response, got %#v", hbResp)
	}
	if hbResp.DirectAttempts[0].IssuedAt.IsZero() {
		t.Fatalf("expected heartbeat direct attempt to include issued_at, got %#v", hbResp.DirectAttempts)
	}

	bootstrapRec := sendJSON(t, handler, http.MethodGet, "/api/v1/nodes/"+first.Node.ID+"/bootstrap", first.NodeToken, nil)
	if bootstrapRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from bootstrap, got %d: %s", bootstrapRec.Code, bootstrapRec.Body.String())
	}
	var bootstrap api.BootstrapConfig
	decodeResponse(t, bootstrapRec, &bootstrap)
	if len(bootstrap.Peers) != 1 {
		t.Fatalf("expected one peer in bootstrap, got %#v", bootstrap.Peers)
	}
	if bootstrap.Peers[0].NATMappingBehavior != "varying_port" || !bootstrap.Peers[0].NATReachable {
		t.Fatalf("expected NAT summary on bootstrap peer, got %#v", bootstrap.Peers[0])
	}
	if bootstrap.Peers[0].ObservedDirectRecoveryLastIssuedAttemptID == "" {
		t.Fatalf("expected bootstrap peer to expose latest issued direct attempt trace, got %#v", bootstrap.Peers[0])
	}
}

func TestRouteOverlapConflict(t *testing.T) {
	handler, cfg, cleanup := newTestServer(t)
	defer cleanup()

	regResp := registerNode(t, handler, cfg)

	firstRoute := api.CreateRouteRequest{
		NetworkCIDR: "10.20.0.0/16",
		ViaNodeID:   regResp.Node.ID,
		Priority:    100,
	}
	firstRec := sendJSON(t, handler, http.MethodPost, "/api/v1/routes", cfg.AdminToken, firstRoute)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected first route create to succeed, got %d: %s", firstRec.Code, firstRec.Body.String())
	}

	secondRoute := api.CreateRouteRequest{
		NetworkCIDR: "10.20.1.0/24",
		ViaNodeID:   regResp.Node.ID,
		Priority:    90,
	}
	secondRec := sendJSON(t, handler, http.MethodPost, "/api/v1/routes", cfg.AdminToken, secondRoute)
	if secondRec.Code != http.StatusConflict {
		t.Fatalf("expected overlap conflict 409, got %d: %s", secondRec.Code, secondRec.Body.String())
	}
}

func newTestServer(t *testing.T) (http.Handler, config.Config, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	cfg := config.Config{
		Address:           ":0",
		StorageDriver:     "sqlite",
		SQLitePath:        filepath.Join(tmpDir, "controlplane.db"),
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
		RelayAddresses:    []string{"relay-ap-1.example.net:3478", "relay-us-1.example.net:3478"},
	}

	dataStore, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}

	return New(cfg, dataStore), cfg, func() {
		_ = dataStore.Close()
		_ = os.RemoveAll(tmpDir)
	}
}

func registerNode(t *testing.T, handler http.Handler, cfg config.Config) api.DeviceRegistrationResponse {
	t.Helper()

	rec := sendJSON(t, handler, http.MethodPost, "/api/v1/devices/register", "", api.DeviceRegistrationRequest{
		DeviceName:        "node-1",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-1",
		RegistrationToken: cfg.RegistrationToken,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected registration success, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp api.DeviceRegistrationResponse
	decodeResponse(t, rec, &resp)
	return resp
}

func sendJSON(t *testing.T, handler http.Handler, method, path, token string, payload any) *httptest.ResponseRecorder {
	t.Helper()

	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		body = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func jsonBody(t *testing.T, payload any) *bytes.Reader {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return bytes.NewReader(raw)
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
