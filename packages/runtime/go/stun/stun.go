package stun

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	bindingRequestType         = 0x0001
	bindingSuccessResponseType = 0x0101
	magicCookie                = 0x2112A442

	attrMappedAddress    = 0x0001
	attrXORMappedAddress = 0x0020
)

type Result struct {
	Server           string `json:"server"`
	Status           string `json:"status"`
	RTTMillis        int64  `json:"rtt_millis,omitempty"`
	ReflexiveAddress string `json:"reflexive_address,omitempty"`
	Error            string `json:"error,omitempty"`
}

type Report struct {
	GeneratedAt              time.Time `json:"generated_at"`
	Reachable                bool      `json:"reachable"`
	SelectedAddress          string    `json:"selected_address,omitempty"`
	SelectedReflexiveAddress string    `json:"selected_reflexive_address,omitempty"`
	MappingBehavior          string    `json:"mapping_behavior,omitempty"`
	SampleCount              int       `json:"sample_count,omitempty"`
	Servers                  []Result  `json:"servers"`
}

type BindingResponse struct {
	TransactionID    string `json:"transaction_id"`
	ReflexiveAddress string `json:"reflexive_address"`
}

func Discover(ctx context.Context, servers []string, timeout time.Duration) (Report, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	report := Report{
		GeneratedAt: time.Now().UTC(),
		Servers:     make([]Result, 0, len(servers)),
	}

	normalizedServers := make([]string, 0, len(servers))
	for _, server := range servers {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		normalizedServers = append(normalizedServers, server)
	}
	if len(normalizedServers) == 0 {
		return report, nil
	}

	var errs []string
	for _, server := range normalizedServers {
		result := Result{Server: server}
		reflexiveAddress, rtt, err := probeServer(ctx, server, timeout)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				result.Status = "timeout"
			} else {
				result.Status = "error"
			}
			result.Error = err.Error()
			errs = append(errs, fmt.Sprintf("%s: %v", server, err))
		} else {
			result.Status = "reachable"
			result.RTTMillis = rtt.Milliseconds()
			result.ReflexiveAddress = reflexiveAddress
			if !report.Reachable {
				report.Reachable = true
				report.SelectedAddress = reflexiveAddress
			}
		}
		report.Servers = append(report.Servers, result)
	}

	finalizeReport(&report)
	if report.Reachable {
		return report, nil
	}
	if len(errs) == 0 {
		return report, errors.New("no stun server configured")
	}
	return report, fmt.Errorf("stun discovery failed: %s", strings.Join(errs, "; "))
}

func FinalizeReport(report Report) Report {
	finalizeReport(&report)
	return report
}

func finalizeReport(report *Report) {
	if report == nil {
		return
	}

	reachable := make([]Result, 0, len(report.Servers))
	for _, result := range report.Servers {
		if result.Status != "reachable" || strings.TrimSpace(result.ReflexiveAddress) == "" {
			continue
		}
		reachable = append(reachable, result)
	}

	report.SampleCount = len(reachable)
	if report.SelectedReflexiveAddress == "" {
		if strings.TrimSpace(report.SelectedAddress) != "" {
			report.SelectedReflexiveAddress = strings.TrimSpace(report.SelectedAddress)
		} else if len(reachable) > 0 {
			report.SelectedReflexiveAddress = reachable[0].ReflexiveAddress
		}
	}
	report.SelectedAddress = report.SelectedReflexiveAddress
	report.MappingBehavior = mappingBehavior(reachable)
}

func mappingBehavior(reachable []Result) string {
	if len(reachable) == 0 {
		return "unknown"
	}

	ports := make(map[string]struct{}, len(reachable))
	for _, result := range reachable {
		host, port, err := net.SplitHostPort(strings.TrimSpace(result.ReflexiveAddress))
		if err != nil || host == "" || port == "" {
			return "unknown"
		}
		if _, err := strconv.Atoi(port); err != nil {
			return "unknown"
		}
		ports[port] = struct{}{}
	}
	if len(ports) <= 1 {
		return "stable_port"
	}
	return "varying_port"
}

func probeServer(ctx context.Context, server string, timeout time.Duration) (string, time.Duration, error) {
	remoteAddress, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return "", 0, fmt.Errorf("resolve stun server: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, remoteAddress)
	if err != nil {
		return "", 0, fmt.Errorf("dial stun server: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return "", 0, fmt.Errorf("set stun deadline: %w", err)
	}

	request, transactionID, err := NewBindingRequest()
	if err != nil {
		return "", 0, err
	}

	sentAt := time.Now().UTC()
	if _, err := conn.Write(request); err != nil {
		return "", 0, fmt.Errorf("write stun request: %w", err)
	}

	buffer := make([]byte, 1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", 0, err
		}
		n, err := conn.Read(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if ctx.Err() != nil {
					return "", 0, ctx.Err()
				}
				return "", 0, context.DeadlineExceeded
			}
			return "", 0, fmt.Errorf("read stun response: %w", err)
		}
		response, err := ParseBindingResponse(buffer[:n])
		if err != nil || response.TransactionID != transactionID {
			continue
		}
		return response.ReflexiveAddress, time.Since(sentAt), nil
	}
}

func NewBindingRequest() ([]byte, string, error) {
	transactionID := make([]byte, 12)
	if _, err := rand.Read(transactionID); err != nil {
		return nil, "", fmt.Errorf("generate stun transaction id: %w", err)
	}

	request := make([]byte, 20)
	binary.BigEndian.PutUint16(request[0:2], bindingRequestType)
	binary.BigEndian.PutUint16(request[2:4], 0)
	binary.BigEndian.PutUint32(request[4:8], magicCookie)
	copy(request[8:20], transactionID)
	return request, hex.EncodeToString(transactionID), nil
}

func ParseBindingResponse(raw []byte) (BindingResponse, error) {
	if len(raw) < 20 {
		return BindingResponse{}, errors.New("stun response too short")
	}
	messageType := binary.BigEndian.Uint16(raw[0:2])
	messageLength := int(binary.BigEndian.Uint16(raw[2:4]))
	if messageType != bindingSuccessResponseType {
		return BindingResponse{}, fmt.Errorf("unexpected stun response type 0x%04x", messageType)
	}
	if binary.BigEndian.Uint32(raw[4:8]) != magicCookie {
		return BindingResponse{}, errors.New("unexpected stun magic cookie")
	}
	if 20+messageLength > len(raw) {
		return BindingResponse{}, errors.New("stun response body truncated")
	}

	transactionID := hex.EncodeToString(raw[8:20])
	body := raw[20 : 20+messageLength]
	for len(body) >= 4 {
		attrType := binary.BigEndian.Uint16(body[0:2])
		attrLength := int(binary.BigEndian.Uint16(body[2:4]))
		if len(body) < 4+attrLength {
			return BindingResponse{}, errors.New("stun attribute truncated")
		}

		value := body[4 : 4+attrLength]
		switch attrType {
		case attrXORMappedAddress:
			address, err := parseXORMappedAddress(value, raw[4:20])
			if err == nil {
				return BindingResponse{
					TransactionID:    transactionID,
					ReflexiveAddress: address,
				}, nil
			}
		case attrMappedAddress:
			address, err := parseMappedAddress(value)
			if err == nil {
				return BindingResponse{
					TransactionID:    transactionID,
					ReflexiveAddress: address,
				}, nil
			}
		}

		paddedLength := attrLength
		if rem := paddedLength % 4; rem != 0 {
			paddedLength += 4 - rem
		}
		if len(body) < 4+paddedLength {
			break
		}
		body = body[4+paddedLength:]
	}

	return BindingResponse{}, errors.New("stun mapped address not found")
}

func parseMappedAddress(value []byte) (string, error) {
	if len(value) < 4 {
		return "", errors.New("mapped address attribute too short")
	}
	family := value[1]
	port := binary.BigEndian.Uint16(value[2:4])
	switch family {
	case 0x01:
		if len(value) < 8 {
			return "", errors.New("mapped ipv4 attribute too short")
		}
		ip := net.IP(value[4:8])
		return net.JoinHostPort(ip.String(), fmt.Sprintf("%d", port)), nil
	case 0x02:
		if len(value) < 20 {
			return "", errors.New("mapped ipv6 attribute too short")
		}
		ip := net.IP(value[4:20])
		return net.JoinHostPort(ip.String(), fmt.Sprintf("%d", port)), nil
	default:
		return "", fmt.Errorf("unsupported mapped address family %d", family)
	}
}

func parseXORMappedAddress(value []byte, header []byte) (string, error) {
	if len(value) < 4 {
		return "", errors.New("xor mapped address attribute too short")
	}
	family := value[1]
	xorPort := binary.BigEndian.Uint16(value[2:4])
	port := xorPort ^ uint16(magicCookie>>16)
	switch family {
	case 0x01:
		if len(value) < 8 {
			return "", errors.New("xor mapped ipv4 attribute too short")
		}
		cookieBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(cookieBytes, magicCookie)
		ip := make([]byte, 4)
		for idx := 0; idx < 4; idx++ {
			ip[idx] = value[4+idx] ^ cookieBytes[idx]
		}
		return net.JoinHostPort(net.IP(ip).String(), fmt.Sprintf("%d", port)), nil
	case 0x02:
		if len(value) < 20 {
			return "", errors.New("xor mapped ipv6 attribute too short")
		}
		if len(header) < 16 {
			return "", errors.New("stun header too short for ipv6 xor address")
		}
		mask := append(make([]byte, 0, 16), header[0:4]...)
		mask = append(mask, header[4:16]...)
		ip := make([]byte, 16)
		for idx := 0; idx < 16; idx++ {
			ip[idx] = value[4+idx] ^ mask[idx]
		}
		return net.JoinHostPort(net.IP(ip).String(), fmt.Sprintf("%d", port)), nil
	default:
		return "", fmt.Errorf("unsupported xor mapped address family %d", family)
	}
}
