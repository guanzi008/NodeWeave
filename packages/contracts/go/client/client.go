package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nodeweave/packages/contracts/go/api"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) WithToken(token string) *Client {
	copyClient := *c
	copyClient.token = token
	return &copyClient
}

func (c *Client) SetToken(token string) {
	c.token = token
}

func (c *Client) Health(ctx context.Context) (api.HealthResponse, error) {
	var resp api.HealthResponse
	err := c.do(ctx, http.MethodGet, "/healthz", "", nil, &resp)
	return resp, err
}

func (c *Client) Login(ctx context.Context, req api.LoginRequest) (api.LoginResponse, error) {
	var resp api.LoginResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/login", "", req, &resp)
	return resp, err
}

func (c *Client) RegisterDevice(ctx context.Context, req api.DeviceRegistrationRequest) (api.DeviceRegistrationResponse, error) {
	var resp api.DeviceRegistrationResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/devices/register", "", req, &resp)
	return resp, err
}

func (c *Client) ListNodes(ctx context.Context) (api.NodeListResponse, error) {
	var resp api.NodeListResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes", c.token, nil, &resp)
	return resp, err
}

func (c *Client) GetBootstrap(ctx context.Context, nodeID, token string) (api.BootstrapConfig, error) {
	var resp api.BootstrapConfig
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/"+nodeID+"/bootstrap", token, nil, &resp)
	return resp, err
}

func (c *Client) Heartbeat(ctx context.Context, nodeID, token string, req api.HeartbeatRequest) (api.HeartbeatResponse, error) {
	var resp api.HeartbeatResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/nodes/"+nodeID+"/heartbeat", token, req, &resp)
	return resp, err
}

func (c *Client) ListRoutes(ctx context.Context) (api.RouteListResponse, error) {
	var resp api.RouteListResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/routes", c.token, nil, &resp)
	return resp, err
}

func (c *Client) CreateRoute(ctx context.Context, req api.CreateRouteRequest) (api.Route, error) {
	var resp api.Route
	err := c.do(ctx, http.MethodPost, "/api/v1/routes", c.token, req, &resp)
	return resp, err
}

func (c *Client) ListDNSZones(ctx context.Context) (api.DNSZoneListResponse, error) {
	var resp api.DNSZoneListResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/dns/zones", c.token, nil, &resp)
	return resp, err
}

func (c *Client) do(ctx context.Context, method, path, token string, reqBody any, out any) error {
	var body io.Reader
	if reqBody != nil {
		raw, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= http.StatusBadRequest {
		var apiErr api.ErrorResponse
		if err := json.NewDecoder(res.Body).Decode(&apiErr); err != nil {
			return fmt.Errorf("http %d", res.StatusCode)
		}
		if apiErr.Error == "" {
			return fmt.Errorf("http %d", res.StatusCode)
		}
		return fmt.Errorf("%s", apiErr.Error)
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
