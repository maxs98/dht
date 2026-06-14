package dht

import (
	"net"
	"testing"
)

func TestInt2Bytes(t *testing.T) {
	cases := []struct {
		in  uint64
		out []byte
	}{
		{0, []byte{0}},
		{1, []byte{1}},
		{256, []byte{1, 0}},
		{22129, []byte{86, 113}},
	}

	for _, c := range cases {
		r := int2bytes(c.in)
		if len(r) != len(c.out) {
			t.Fail()
		}

		for i, v := range r {
			if v != c.out[i] {
				t.Fail()
			}
		}
	}
}

func TestBytes2Int(t *testing.T) {
	cases := []struct {
		in  []byte
		out uint64
	}{
		{[]byte{0}, 0},
		{[]byte{1}, 1},
		{[]byte{1, 0}, 256},
		{[]byte{86, 113}, 22129},
	}

	for _, c := range cases {
		if bytes2int(c.in) != c.out {
			t.Fail()
		}
	}
}

func TestDecodeCompactIPPortInfo(t *testing.T) {
	cases := []struct {
		in  string
		out struct {
			ip   string
			port int
		}
	}{
		{"123456", struct {
			ip   string
			port int
		}{"49.50.51.52", 13622}},
		{"abcdef", struct {
			ip   string
			port int
		}{"97.98.99.100", 25958}},
		// IPv6: 18 bytes
		{string([]byte{
			0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
			0x1a, 0xe1,
		}), struct {
			ip   string
			port int
		}{"2001:db8::1", 6881}},
	}

	for _, item := range cases {
		ip, port, err := decodeCompactIPPortInfo(item.in)
		if err != nil || ip.String() != item.out.ip || port != item.out.port {
			t.Errorf("decodeCompactIPPortInfo(%q) = (%s, %d), expected (%s, %d), err=%v",
				item.in, ip.String(), port, item.out.ip, item.out.port, err)
		}
	}
}

func TestEncodeCompactIPPortInfo(t *testing.T) {
	cases := []struct {
		in struct {
			ip   []byte
			port int
		}
		out string
	}{
		{struct {
			ip   []byte
			port int
		}{[]byte{49, 50, 51, 52}, 13622}, "123456"},
		{struct {
			ip   []byte
			port int
		}{[]byte{97, 98, 99, 100}, 25958}, "abcdef"},
		// IPv6
		{struct {
			ip   []byte
			port int
		}{[]byte{
			0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		}, 6881}, string([]byte{
			0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
			0x1a, 0xe1,
		})},
	}

	for _, item := range cases {
		info, err := encodeCompactIPPortInfo(item.in.ip, item.in.port)
		if err != nil || info != item.out {
			t.Errorf("encodeCompactIPPortInfo(%v, %d) = %q (len=%d), expected %q (len=%d), err=%v",
				item.in.ip, item.in.port, info, len(info), item.out, len(item.out), err)
		}
	}
}

func TestIPv6NodeCompactInfo(t *testing.T) {
	// Build an IPv6 node and check compact info is 38 bytes
	id := randomString(20)
	ip := []byte{
		0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
	}
	no := &node{
		id: newBitmapFromString(id),
		addr: &net.UDPAddr{
			IP:   ip,
			Port: 6881,
		},
	}

	compact := no.CompactNodeInfo()
	if len(compact) != 38 {
		t.Errorf("IPv6 CompactNodeInfo should be 38 bytes, got %d", len(compact))
	}

	// Parse it back
	no2, err := newNodeFromCompactInfo(compact, "udp6")
	if err != nil {
		t.Fatalf("newNodeFromCompactInfo IPv6 failed: %v", err)
	}
	if !no2.addr.IP.Equal(no.addr.IP) {
		t.Errorf("IPv6 addr mismatch: %s vs %s", no2.addr.IP, no.addr.IP)
	}
	if no2.addr.Port != no.addr.Port {
		t.Errorf("port mismatch: %d vs %d", no2.addr.Port, no.addr.Port)
	}
}

func TestIPv4NodeCompactInfo(t *testing.T) {
	id := randomString(20)
	no := &node{
		id: newBitmapFromString(id),
		addr: &net.UDPAddr{
			IP:   net.IPv4(1, 2, 3, 4),
			Port: 6881,
		},
	}

	compact := no.CompactNodeInfo()
	if len(compact) != 26 {
		t.Errorf("IPv4 CompactNodeInfo should be 26 bytes, got %d", len(compact))
	}

	no2, err := newNodeFromCompactInfo(compact, "udp4")
	if err != nil {
		t.Fatalf("newNodeFromCompactInfo IPv4 failed: %v", err)
	}
	if !no2.addr.IP.Equal(no.addr.IP) {
		t.Errorf("IPv4 addr mismatch: %s vs %s", no2.addr.IP, no.addr.IP)
	}
}

func TestGenAddress(t *testing.T) {
	tests := []struct{ ip string; port int; expected string }{
		{"1.2.3.4", 6881, "1.2.3.4:6881"},
		{"::1", 6881, "[::1]:6881"},
		{"2001:db8::1", 80, "[2001:db8::1]:80"},
		{"192.168.1.1", 0, "192.168.1.1:0"},
	}
	for _, tc := range tests {
		got := genAddress(tc.ip, tc.port)
		if got != tc.expected {
			t.Errorf("genAddress(%s, %d) = %s, expected %s", tc.ip, tc.port, got, tc.expected)
		}
	}
}

func TestIPv6Config(t *testing.T) {
	cfg := NewStandardConfig6()
	if cfg.Network != "udp6" {
		t.Errorf("expected udp6, got %s", cfg.Network)
	}
	if cfg.Mode != StandardMode {
		t.Error("expected StandardMode")
	}
}
