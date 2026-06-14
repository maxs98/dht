package dht

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestParseSTUN(t *testing.T) {
	// Build a minimal STUN binding response with XOR-MAPPED-ADDRESS
	// Header(20) + attr type(2) + attr len(2) + value(8) + padding(4) = 36
	buf := make([]byte, 36)
	binary.BigEndian.PutUint16(buf[0:2], stunBindingResponse)
	binary.BigEndian.PutUint16(buf[2:4], 12) // length
	binary.BigEndian.PutUint32(buf[4:8], stunMagicCookie)
	// transaction ID (random)
	copy(buf[8:20], []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11})

	// XOR-MAPPED-ADDRESS attribute
	binary.BigEndian.PutUint16(buf[20:22], stunAttrXorMappedAddress)
	binary.BigEndian.PutUint16(buf[22:24], 8) // length

	// Value: family=IPv4(0x01), port=1234, IP=1.2.3.4
	ip := net.IP{1, 2, 3, 4}
	port := uint16(1234)

	buf[24] = 0    // reserved
	buf[25] = 0x01 // IPv4
	// XOR port with magic cookie high 2 bytes
	xorPort := port ^ uint16(stunMagicCookie>>16)
	binary.BigEndian.PutUint16(buf[26:28], xorPort)
	// XOR IP with magic cookie
	rawIP := binary.BigEndian.Uint32(ip)
	xorIP := rawIP ^ stunMagicCookie
	binary.BigEndian.PutUint32(buf[28:32], xorIP)
	// Padding (4 bytes to align)
	for i := 32; i < 36; i++ {
		buf[i] = 0
	}

	msg, err := parseSTUN(buf[:36])
	if err != nil {
		t.Fatalf("parseSTUN failed: %v", err)
	}

	parsedIP, parsedPort, err := msg.decodeXORMappedAddress()
	if err != nil {
		t.Fatalf("decodeXORMappedAddress failed: %v", err)
	}

	if !parsedIP.Equal(ip) {
		t.Errorf("expected IP %s, got %s", ip, parsedIP)
	}
	if parsedPort != int(port) {
		t.Errorf("expected port %d, got %d", port, parsedPort)
	}
}

func TestParseSTUNMappedAddress(t *testing.T) {
	// Test fallback to MAPPED-ADDRESS (non-XOR)
	buf := make([]byte, 20+12)
	binary.BigEndian.PutUint16(buf[0:2], stunBindingResponse)
	binary.BigEndian.PutUint16(buf[2:4], 12)
	binary.BigEndian.PutUint32(buf[4:8], stunMagicCookie)

	// MAPPED-ADDRESS (not XOR'd)
	binary.BigEndian.PutUint16(buf[20:22], stunAttrMappedAddress)
	binary.BigEndian.PutUint16(buf[22:24], 8)
	buf[24] = 0
	buf[25] = 0x01 // IPv4
	binary.BigEndian.PutUint16(buf[26:28], 5678) // port (not XOR'd)
	binary.BigEndian.PutUint32(buf[28:32], binary.BigEndian.Uint32(net.IP{10, 20, 30, 40}))

	msg, err := parseSTUN(buf[:32])
	if err != nil {
		t.Fatalf("parseSTUN failed: %v", err)
	}

	parsedIP, parsedPort, err := msg.decodeXORMappedAddress()
	if err != nil {
		t.Fatalf("decodeXORMappedAddress failed: %v", err)
	}

	if !parsedIP.Equal(net.IP{10, 20, 30, 40}) {
		t.Errorf("expected 10.20.30.40, got %s", parsedIP)
	}
	if parsedPort != 5678 {
		t.Errorf("expected port 5678, got %d", parsedPort)
	}
}

func TestParseSTUNInvalid(t *testing.T) {
	// Too short
	_, err := parseSTUN(make([]byte, 10))
	if err == nil {
		t.Error("expected error for short message")
	}

	// Wrong magic cookie
	buf := make([]byte, 20)
	binary.BigEndian.PutUint32(buf[4:8], 0xDEADBEEF)
	_, err = parseSTUN(buf)
	if err == nil {
		t.Error("expected error for bad magic cookie")
	}
}

func TestNATConfigFactories(t *testing.T) {
	// Test NewNATCrawlConfig
	cfg := NewNATCrawlConfig()
	if cfg.NATConfig == nil {
		t.Fatal("expected NATConfig to be set")
	}
	if !cfg.NATConfig.Enabled {
		t.Error("expected NAT traversal to be enabled")
	}
	if len(cfg.NATConfig.STUNServers) == 0 {
		t.Error("expected default STUN servers")
	}
	if cfg.Mode != CrawlMode {
		t.Error("expected CrawlMode")
	}

	// Test NewNATCrawlConfigWithIP
	cfg2 := NewNATCrawlConfigWithIP("1.2.3.4", 9999)
	if cfg2.NATConfig == nil {
		t.Fatal("expected NATConfig to be set")
	}
	if cfg2.NATConfig.PublicIP != "1.2.3.4" {
		t.Errorf("expected PublicIP 1.2.3.4, got %s", cfg2.NATConfig.PublicIP)
	}
	if cfg2.NATConfig.PublicPort != 9999 {
		t.Errorf("expected PublicPort 9999, got %d", cfg2.NATConfig.PublicPort)
	}
}

func TestDiscoverNATManualOverride(t *testing.T) {
	cfg := NewCrawlConfig()
	cfg.NATConfig = &NATConfig{
		Enabled:     true,
		PublicIP:    "8.8.8.8",
		PublicPort:  12345,
	}

	d := New(cfg)
	// Initialize enough for discoverNAT to work
	localAddr, _ := net.ResolveUDPAddr("udp", cfg.Address)
	d.node.addr = localAddr
	d.discoverNAT()

	if d.natInfo == nil {
		t.Fatal("expected natInfo to be populated")
	}
	if !d.natInfo.PublicIP.Equal(net.IP{8, 8, 8, 8}) {
		t.Errorf("expected public IP 8.8.8.8, got %s", d.natInfo.PublicIP)
	}
	if d.natInfo.PublicPort != 12345 {
		t.Errorf("expected public port 12345, got %d", d.natInfo.PublicPort)
	}
}

func TestPublicAddr(t *testing.T) {
	cfg := NewCrawlConfig()
	cfg.NATConfig = &NATConfig{
		Enabled:     true,
		PublicIP:    "10.0.0.1",
		PublicPort:  6881,
	}

	d := New(cfg)
	localAddr, _ := net.ResolveUDPAddr("udp", cfg.Address)
	d.node.addr = localAddr
	d.discoverNAT()

	pub := d.PublicAddr()
	if pub == nil {
		t.Fatal("expected PublicAddr to return non-nil")
	}
	if !pub.IP.Equal(net.IP{10, 0, 0, 1}) {
		t.Errorf("expected 10.0.0.1, got %s", pub.IP)
	}
	if pub.Port != 6881 {
		t.Errorf("expected port 6881, got %d", pub.Port)
	}

	// Without NAT: should return local addr
	cfg2 := NewCrawlConfig()
	d2 := New(cfg2)
	pub2 := d2.PublicAddr()
	if pub2 == nil {
		t.Fatal("expected non-nil even without NAT")
	}
}
