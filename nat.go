package dht

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"
)

// STUN magic cookie (RFC 5389)
const stunMagicCookie = 0x2112A442

// STUN message types
const (
	stunBindingRequest  = 0x0001
	stunBindingResponse = 0x0101
)

// STUN attribute types
const (
	stunAttrMappedAddress    = 0x0001
	stunAttrXorMappedAddress = 0x0020
)

// Default public STUN servers (IPv4)
var DefaultSTUNServers = []string{
	"stun.l.google.com:19302",
	"stun1.l.google.com:19302",
	"stun2.l.google.com:19302",
}

// DefaultSTUNServers6 are IPv6-capable STUN servers.
var DefaultSTUNServers6 = []string{
	"stun.l.google.com:19302",
	"stun1.l.google.com:19302",
}

// NATConfig holds NAT traversal configuration.
type NATConfig struct {
	Enabled     bool
	STUNServers []string
	// PublicIP is a manual override — if set, STUN is skipped.
	PublicIP string
	// PublicPort is a manual port override.
	PublicPort int
}

// NATInfo holds the discovered NAT mapping.
type NATInfo struct {
	PublicIP   net.IP
	PublicPort int
	LocalAddr  *net.UDPAddr
}

// stunMessage represents a parsed STUN message.
type stunMessage struct {
	msgType    uint16
	msgLen     uint16
	magic      uint32
	transID    [12]byte
	attributes map[uint16][]byte
}

// parseSTUN attempts to parse raw bytes as a STUN message.
func parseSTUN(data []byte) (*stunMessage, error) {
	if len(data) < 20 {
		return nil, errors.New("STUN message too short")
	}

	msg := &stunMessage{
		msgType:    binary.BigEndian.Uint16(data[0:2]),
		msgLen:     binary.BigEndian.Uint16(data[2:4]),
		magic:      binary.BigEndian.Uint32(data[4:8]),
		attributes: make(map[uint16][]byte),
	}

	if msg.magic != stunMagicCookie {
		return nil, fmt.Errorf("invalid STUN magic cookie: 0x%x", msg.magic)
	}

	copy(msg.transID[:], data[8:20])

	// Parse attributes
	offset := 20
	end := 20 + int(msg.msgLen)
	if end > len(data) {
		end = len(data)
	}

	for offset+4 <= end {
		attrType := binary.BigEndian.Uint16(data[offset : offset+2])
		attrLen := binary.BigEndian.Uint16(data[offset+2 : offset+4])
		offset += 4

		attrEnd := offset + int(attrLen)
		if attrEnd > end {
			break
		}

		if attrLen > 0 {
			msg.attributes[attrType] = data[offset:attrEnd]
			// Pad to 4-byte boundary
			padding := (4 - (int(attrLen) % 4)) % 4
			offset = attrEnd + padding
		} else {
			// Zero-length attributes still advance past the 4-byte header
		}
	}

	return msg, nil
}

// decodeXORMappedAddress decodes XOR-MAPPED-ADDRESS or MAPPED-ADDRESS attribute.
// XOR-MAPPED-ADDRESS values are XOR'd with the magic cookie;
// MAPPED-ADDRESS values are plain.
func (m *stunMessage) decodeXORMappedAddress() (net.IP, int, error) {
	var data []byte
	xor := true

	data, ok := m.attributes[stunAttrXorMappedAddress]
	if !ok {
		data, ok = m.attributes[stunAttrMappedAddress]
		xor = false
		if !ok {
			return nil, 0, errors.New("no mapped address in STUN response")
		}
	}

	if len(data) < 8 {
		return nil, 0, fmt.Errorf("mapped address too short: %d bytes", len(data))
	}

	// First byte: 0 (reserved) | Second byte: family (0x01 = IPv4)
	family := data[1]
	if family != 0x01 {
		return nil, 0, fmt.Errorf("unsupported address family: %d", family)
	}

	port := binary.BigEndian.Uint16(data[2:4])
	rawIP := binary.BigEndian.Uint32(data[4:8])

	if xor {
		port ^= uint16(stunMagicCookie >> 16)
		rawIP ^= stunMagicCookie
	}

	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, rawIP)

	return ip, int(port), nil
}

// stunBind sends a STUN binding request and returns the mapped address.
func stunBind(stunServer string, localAddr *net.UDPAddr, timeout time.Duration) (net.IP, int, error) {
	raddr, err := net.ResolveUDPAddr("udp", stunServer)
	if err != nil {
		return nil, 0, fmt.Errorf("resolve STUN server %s: %w", stunServer, err)
	}

	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		return nil, 0, fmt.Errorf("listen UDP for STUN: %w", err)
	}
	defer conn.Close()

	return stunBindWithConn(conn, raddr, timeout)
}

// stunBindWithConn sends a STUN binding request using an existing UDP connection.
// This allows STUN to use the DHT's own socket, so the returned public address
// correctly reflects the DHT port's NAT mapping (not an ephemeral port).
func stunBindWithConn(conn *net.UDPConn, raddr *net.UDPAddr, timeout time.Duration) (net.IP, int, error) {
	// Build STUN binding request
	request := make([]byte, 20)
	binary.BigEndian.PutUint16(request[0:2], stunBindingRequest)
	binary.BigEndian.PutUint32(request[4:8], stunMagicCookie)
	transID := make([]byte, 12)
	rand.Read(transID)
	copy(request[8:20], transID)

	_, err := conn.WriteToUDP(request, raddr)
	if err != nil {
		return nil, 0, fmt.Errorf("send STUN request: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 1500)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, 0, fmt.Errorf("read STUN response: %w", err)
	}

	msg, err := parseSTUN(buf[:n])
	if err != nil {
		return nil, 0, fmt.Errorf("parse STUN response: %w", err)
	}

	if msg.msgType != stunBindingResponse {
		return nil, 0, fmt.Errorf("unexpected STUN response type: 0x%x", msg.msgType)
	}

	ip, port, err := msg.decodeXORMappedAddress()
	if err != nil {
		return nil, 0, fmt.Errorf("decode mapped address: %w", err)
	}

	return ip, port, nil
}

// DiscoverNAT probes STUN servers to discover the public IP:port mapping.
// Uses a random local port for the STUN socket (must not conflict with the
// DHT listener which already holds the configured port).
// Prefer DiscoverNATWithConn when you have the DHT socket — it returns
// the correct public port mapping for the DHT port.
func DiscoverNAT(servers []string, localAddr string, timeout time.Duration) (*NATInfo, error) {
	if len(servers) == 0 {
		servers = DefaultSTUNServers
	}

	addr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return nil, fmt.Errorf("resolve local address: %w", err)
	}

	var lastErr error
	for _, server := range servers {
		ip, port, err := stunBind(server, addr, timeout)
		if err != nil {
			lastErr = err
			continue
		}
		return &NATInfo{
			PublicIP:   ip,
			PublicPort: port,
			LocalAddr:  addr,
		}, nil
	}

	return nil, fmt.Errorf("all STUN servers failed, last error: %w", lastErr)
}

// DiscoverNATWithConn probes STUN servers using an existing UDP connection.
// This is the preferred method for DHT — it uses the DHT's own socket,
// so the returned public port correctly reflects the DHT port's NAT mapping.
func DiscoverNATWithConn(conn *net.UDPConn, servers []string, timeout time.Duration) (*NATInfo, error) {
	if len(servers) == 0 {
		servers = DefaultSTUNServers
	}

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	var lastErr error
	for _, server := range servers {
		raddr, err := net.ResolveUDPAddr("udp", server)
		if err != nil {
			lastErr = err
			continue
		}
		ip, port, err := stunBindWithConn(conn, raddr, timeout)
		if err != nil {
			lastErr = err
			continue
		}
		return &NATInfo{
			PublicIP:   ip,
			PublicPort: port,
			LocalAddr:  localAddr,
		}, nil
	}

	return nil, fmt.Errorf("all STUN servers failed, last error: %w", lastErr)
}
