package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/services/controlplane/internal/config"
	"nodeweave/services/controlplane/internal/store"
)

type Server struct {
	cfg   config.Config
	store store.Store
	mux   *http.ServeMux
}

func New(cfg config.Config, dataStore store.Store) http.Handler {
	server := &Server{
		cfg:   cfg,
		store: dataStore,
		mux:   http.NewServeMux(),
	}
	server.routes()
	return server.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/v1/devices/register", s.handleRegisterDevice)
	s.mux.HandleFunc("GET /api/v1/nodes", s.handleListNodes)
	s.mux.HandleFunc("GET /api/v1/nodes/{id}/bootstrap", s.handleBootstrap)
	s.mux.HandleFunc("POST /api/v1/nodes/{id}/heartbeat", s.handleHeartbeat)
	s.mux.HandleFunc("GET /api/v1/routes", s.handleListRoutes)
	s.mux.HandleFunc("POST /api/v1/routes", s.handleCreateRoute)
	s.mux.HandleFunc("GET /api/v1/dns/zones", s.handleListZones)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: "ok"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req api.LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !s.store.ValidateAdminCredentials(req.Email, req.Password) {
		writeError(w, http.StatusUnauthorized, store.ErrUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, api.LoginResponse{
		AccessToken: s.cfg.AdminToken,
		TokenType:   "Bearer",
		User:        s.store.AdminUser(),
	})
}

func (s *Server) handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	var req api.DeviceRegistrationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := s.store.CreateDeviceAndNode(req)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminRequest(r) {
		writeError(w, http.StatusUnauthorized, store.ErrUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, api.NodeListResponse{Items: s.store.ListNodes()})
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	bootstrap, err := s.store.GetBootstrap(r.PathValue("id"), bearerToken(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bootstrap)
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req api.HeartbeatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := s.store.UpdateHeartbeat(r.PathValue("id"), bearerToken(r), req)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListRoutes(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminRequest(r) {
		writeError(w, http.StatusUnauthorized, store.ErrUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, api.RouteListResponse{Items: s.store.ListRoutes()})
}

func (s *Server) handleCreateRoute(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminRequest(r) {
		writeError(w, http.StatusUnauthorized, store.ErrUnauthorized)
		return
	}

	var req api.CreateRouteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	route, err := s.store.CreateRoute(req)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, route)
}

func (s *Server) handleListZones(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminRequest(r) {
		writeError(w, http.StatusUnauthorized, store.ErrUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, api.DNSZoneListResponse{Items: s.store.ListDNSZones()})
}

func (s *Server) isAdminRequest(r *http.Request) bool {
	return bearerToken(r) == s.cfg.AdminToken
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, err)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, store.ErrInvalid):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, api.ErrorResponse{Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return ""
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
