package store

import "nodeweave/packages/contracts/go/api"

type Store interface {
	ValidateAdminCredentials(email, password string) bool
	AdminUser() api.User
	CreateDeviceAndNode(req api.DeviceRegistrationRequest) (api.DeviceRegistrationResponse, error)
	UpdateHeartbeat(nodeID, token string, req api.HeartbeatRequest) (api.HeartbeatResponse, error)
	GetBootstrap(nodeID, token string) (api.BootstrapConfig, error)
	ListNodes() []api.Node
	ListRoutes() []api.Route
	CreateRoute(req api.CreateRouteRequest) (api.Route, error)
	ListDNSZones() []api.DNSZone
	Close() error
}
