package linuxexec

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os/exec"
	"strings"
	"time"

	linuxplan "nodeweave/packages/runtime/go/plan/linux"
)

type Inspector interface {
	InterfaceState(context.Context, string, time.Duration) (InterfaceState, error)
	AddressAssigned(context.Context, string, string, time.Duration) (bool, error)
	RouteState(context.Context, string, time.Duration) (RouteState, error)
	LinkNameservers(context.Context, string, time.Duration) ([]string, error)
	LinkDomains(context.Context, string, time.Duration) ([]string, error)
}

type InterfaceState struct {
	Exists bool
	Up     bool
	MTU    int
}

type RouteState struct {
	Exists bool
	Device string
}

type OSInspector struct{}

func (OSInspector) InterfaceState(ctx context.Context, ifName string, timeout time.Duration) (InterfaceState, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "ip", "-json", "link", "show", "dev", ifName)
	output, err := cmd.Output()
	if err != nil {
		if isNotFoundError(err) {
			return InterfaceState{}, nil
		}
		return InterfaceState{}, fmt.Errorf("inspect interface: %w", err)
	}

	var payload []struct {
		IfName string   `json:"ifname"`
		MTU    int      `json:"mtu"`
		Flags  []string `json:"flags"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return InterfaceState{}, fmt.Errorf("parse interface state: %w", err)
	}
	if len(payload) == 0 {
		return InterfaceState{}, nil
	}

	state := InterfaceState{
		Exists: true,
		MTU:    payload[0].MTU,
	}
	for _, flag := range payload[0].Flags {
		if strings.EqualFold(flag, "UP") {
			state.Up = true
			break
		}
	}
	return state, nil
}

func (OSInspector) AddressAssigned(ctx context.Context, ifName, cidr string, timeout time.Duration) (bool, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "ip", "-json", "addr", "show", "dev", ifName)
	output, err := cmd.Output()
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect interface addresses: %w", err)
	}

	wantPrefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return false, fmt.Errorf("parse desired cidr: %w", err)
	}

	var payload []struct {
		AddrInfo []struct {
			Local     string `json:"local"`
			PrefixLen int    `json:"prefixlen"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return false, fmt.Errorf("parse interface addresses: %w", err)
	}

	for _, link := range payload {
		for _, addr := range link.AddrInfo {
			parsedAddr, err := netip.ParseAddr(addr.Local)
			if err != nil {
				continue
			}
			if netip.PrefixFrom(parsedAddr, addr.PrefixLen) == wantPrefix {
				return true, nil
			}
		}
	}
	return false, nil
}

func (OSInspector) RouteState(ctx context.Context, cidr string, timeout time.Duration) (RouteState, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "ip", "-json", "route", "show", "exact", cidr)
	output, err := cmd.Output()
	if err != nil {
		return RouteState{}, fmt.Errorf("inspect route: %w", err)
	}

	var payload []struct {
		Dst string `json:"dst"`
		Dev string `json:"dev"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return RouteState{}, fmt.Errorf("parse route state: %w", err)
	}
	if len(payload) == 0 {
		return RouteState{}, nil
	}
	return RouteState{
		Exists: true,
		Device: payload[0].Dev,
	}, nil
}

func (OSInspector) LinkNameservers(ctx context.Context, ifName string, timeout time.Duration) ([]string, error) {
	return inspectResolvectlFields(ctx, timeout, "dns", ifName, false)
}

func (OSInspector) LinkDomains(ctx context.Context, ifName string, timeout time.Duration) ([]string, error) {
	return inspectResolvectlFields(ctx, timeout, "domain", ifName, true)
}

func operationSatisfied(ctx context.Context, inspector Inspector, operation linuxplan.Operation, timeout time.Duration) (bool, error) {
	switch operation.Kind {
	case "interface_create":
		state, err := inspector.InterfaceState(ctx, operation.Interface, timeout)
		if err != nil {
			return false, err
		}
		return state.Exists, nil
	case "interface_link":
		state, err := inspector.InterfaceState(ctx, operation.Interface, timeout)
		if err != nil {
			return false, err
		}
		return state.Exists && state.Up && state.MTU == operation.MTU, nil
	case "interface_address":
		return inspector.AddressAssigned(ctx, operation.Interface, operation.AddressCIDR, timeout)
	case "route_replace":
		state, err := inspector.RouteState(ctx, operation.RouteCIDR, timeout)
		if err != nil {
			return false, err
		}
		return state.Exists && state.Device == operation.RouteDev, nil
	case "dns_servers":
		nameservers, err := inspector.LinkNameservers(ctx, operation.Interface, timeout)
		if err != nil {
			return false, err
		}
		return sameValues(nameservers, operation.Nameservers), nil
	case "dns_domain":
		domains, err := inspector.LinkDomains(ctx, operation.Interface, timeout)
		if err != nil {
			return false, err
		}
		wantDomain := strings.TrimSpace(operation.Domain)
		for _, domain := range domains {
			if domain == wantDomain {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, nil
	}
}

func isNotFoundError(err error) bool {
	return strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "Cannot find device")
}

func inspectResolvectlFields(ctx context.Context, timeout time.Duration, verb, ifName string, stripRouteOnly bool) ([]string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "resolvectl", verb, ifName)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("inspect resolvectl %s: %w", verb, err)
	}

	line := strings.TrimSpace(string(output))
	if line == "" {
		return []string{}, nil
	}
	if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
		line = strings.TrimSpace(parts[1])
	}
	if line == "" {
		return []string{}, nil
	}

	fields := strings.Fields(line)
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if stripRouteOnly {
			field = strings.TrimPrefix(field, "~")
		}
		values = append(values, field)
	}
	return values, nil
}

func sameValues(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	remaining := make(map[string]int, len(want))
	for _, value := range want {
		remaining[strings.TrimSpace(value)]++
	}
	for _, value := range got {
		value = strings.TrimSpace(value)
		if remaining[value] == 0 {
			return false
		}
		remaining[value]--
	}
	for _, count := range remaining {
		if count != 0 {
			return false
		}
	}
	return true
}
