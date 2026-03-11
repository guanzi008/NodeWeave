package stun

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestDiscoverReturnsReflexiveAddress(t *testing.T) {
	server := newTestSTUNServer(t, 0)
	defer server.Close()

	report, err := Discover(context.Background(), []string{server.Address()}, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("discover stun: %v", err)
	}
	if !report.Reachable {
		t.Fatalf("expected reachable report, got %#v", report)
	}
	if report.SelectedAddress == "" {
		t.Fatalf("expected selected address, got %#v", report)
	}
	if len(report.Servers) != 1 || report.Servers[0].Status != "reachable" {
		t.Fatalf("unexpected server results %#v", report.Servers)
	}
}

func TestDiscoverReportsTimeout(t *testing.T) {
	server := newTestSTUNServer(t, 300*time.Millisecond)
	defer server.Close()

	report, err := Discover(context.Background(), []string{server.Address()}, 50*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error, got report %#v", report)
	}
	if report.Reachable {
		t.Fatalf("expected unreachable report, got %#v", report)
	}
	if len(report.Servers) != 1 || report.Servers[0].Status != "timeout" {
		t.Fatalf("unexpected timeout server results %#v", report.Servers)
	}
}

type testSTUNServer struct {
	conn  net.PacketConn
	delay time.Duration
}

func newTestSTUNServer(t *testing.T, delay time.Duration) *testSTUNServer {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen test stun server: %v", err)
	}
	server := &testSTUNServer{
		conn:  conn,
		delay: delay,
	}
	go server.serve(t)
	return server
}

func (s *testSTUNServer) Address() string {
	return s.conn.LocalAddr().String()
}

func (s *testSTUNServer) Close() {
	_ = s.conn.Close()
}

func (s *testSTUNServer) serve(t *testing.T) {
	t.Helper()

	buffer := make([]byte, 1024)
	for {
		n, addr, err := s.conn.ReadFrom(buffer)
		if err != nil {
			return
		}
		if n < 20 {
			continue
		}
		transactionID := append([]byte(nil), buffer[8:20]...)
		if s.delay > 0 {
			time.Sleep(s.delay)
		}
		response, err := buildTestBindingSuccess(transactionID, addr)
		if err != nil {
			t.Errorf("build test stun response: %v", err)
			return
		}
		if _, err := s.conn.WriteTo(response, addr); err != nil {
			return
		}
	}
}

func buildTestBindingSuccess(transactionID []byte, addr net.Addr) ([]byte, error) {
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return nil, net.InvalidAddrError("non-udp address")
	}
	ip4 := udpAddr.IP.To4()
	if ip4 == nil {
		return nil, net.InvalidAddrError("non-ipv4 address")
	}

	attr := make([]byte, 12)
	binary.BigEndian.PutUint16(attr[0:2], attrXORMappedAddress)
	binary.BigEndian.PutUint16(attr[2:4], 8)
	attr[4] = 0
	attr[5] = 0x01
	binary.BigEndian.PutUint16(attr[6:8], uint16(udpAddr.Port)^uint16(magicCookie>>16))
	cookieBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cookieBytes, magicCookie)
	for idx := 0; idx < 4; idx++ {
		attr[8+idx] = ip4[idx] ^ cookieBytes[idx]
	}

	message := make([]byte, 20+len(attr))
	binary.BigEndian.PutUint16(message[0:2], bindingSuccessResponseType)
	binary.BigEndian.PutUint16(message[2:4], uint16(len(attr)))
	binary.BigEndian.PutUint32(message[4:8], magicCookie)
	copy(message[8:20], transactionID)
	copy(message[20:], attr)
	return message, nil
}
