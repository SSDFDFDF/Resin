package outbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Resinat/Resin/internal/testutil"
	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
)

// ---------------------------------------------------------------------------
// SingboxBuilder constructor / teardown
// ---------------------------------------------------------------------------

func TestNewSingboxBuilder(t *testing.T) {
	b, err := NewSingboxBuilder()
	if err != nil {
		t.Fatalf("NewSingboxBuilder() error: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("SingboxBuilder.Close() error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Build: parse and create real outbound
// ---------------------------------------------------------------------------

func TestSingboxBuilder_ParseShadowsocks(t *testing.T) {
	b, err := NewSingboxBuilder()
	if err != nil {
		t.Fatalf("NewSingboxBuilder() error: %v", err)
	}
	defer b.Close()

	raw := json.RawMessage(`{
		"type": "shadowsocks",
		"tag":  "test-ss",
		"server": "127.0.0.1",
		"server_port": 8388,
		"method": "aes-256-gcm",
		"password": "test-password"
	}`)
	ob, err := b.Build(raw)
	if err != nil {
		t.Fatalf("Build(shadowsocks) error: %v", err)
	}

	// Should implement io.Closer (sing-box outbounds do)
	closer, ok := ob.(io.Closer)
	if !ok {
		t.Fatal("expected outbound to implement io.Closer")
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("outbound Close() error: %v", err)
	}
}

func TestSingboxBuilder_ParseExtendedProtocols(t *testing.T) {
	b, err := NewSingboxBuilder()
	if err != nil {
		t.Fatalf("NewSingboxBuilder() error: %v", err)
	}
	defer b.Close()

	cases := []struct {
		name               string
		raw                json.RawMessage
		missingFeatureHint string
	}{
		{
			name: "socks",
			raw: json.RawMessage(`{
				"type":"socks",
				"tag":"test-socks",
				"server":"127.0.0.1",
				"server_port":1080,
				"version":"5",
				"username":"user",
				"password":"pass"
			}`),
		},
		{
			name: "http",
			raw: json.RawMessage(`{
				"type":"http",
				"tag":"test-http",
				"server":"127.0.0.1",
				"server_port":8080,
				"username":"user",
				"password":"pass"
			}`),
		},
		{
			name: "wireguard",
			raw: json.RawMessage(`{
				"type":"wireguard",
				"tag":"test-wg",
				"server":"127.0.0.1",
				"server_port":2480,
				"local_address":["172.16.0.2/32","fd01::1/128"],
				"private_key":"eCtXsJZ27+4PbhDkHnB923tkUn2Gj59wZw5wFA75MnU=",
				"peer_public_key":"Cr8hWlKvtDt7nrvf+f0brNQQzabAqrjfBvas9pmowjo="
			}`),
			missingFeatureHint: "WireGuard is not included in this build",
		},
		{
			name: "hysteria",
			raw: json.RawMessage(`{
				"type":"hysteria",
				"tag":"test-hysteria",
				"server":"127.0.0.1",
				"server_port":443,
				"auth_str":"password",
				"up_mbps":30,
				"down_mbps":200,
				"tls":{"enabled":true,"insecure":true,"server_name":"example.com"}
			}`),
			missingFeatureHint: "QUIC is not included in this build",
		},
		{
			name: "tuic",
			raw: json.RawMessage(`{
				"type":"tuic",
				"tag":"test-tuic",
				"server":"127.0.0.1",
				"server_port":443,
				"uuid":"00000000-0000-0000-0000-000000000001",
				"password":"password",
				"tls":{"enabled":true,"insecure":true,"server_name":"example.com"}
			}`),
			missingFeatureHint: "QUIC is not included in this build",
		},
		{
			name: "anytls",
			raw: json.RawMessage(`{
				"type":"anytls",
				"tag":"test-anytls",
				"server":"127.0.0.1",
				"server_port":443,
				"password":"password",
				"tls":{"enabled":true,"insecure":true,"server_name":"example.com"}
			}`),
		},
		{
			name: "ssh",
			raw: json.RawMessage(`{
				"type":"ssh",
				"tag":"test-ssh",
				"server":"127.0.0.1",
				"server_port":22,
				"user":"root",
				"password":"password"
			}`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ob, err := b.Build(tc.raw)
			if err != nil {
				if tc.missingFeatureHint != "" && strings.Contains(err.Error(), tc.missingFeatureHint) {
					t.Skipf("skipping %s: %v", tc.name, err)
					return
				}
				t.Fatalf("Build(%s) error: %v", tc.name, err)
			}
			if ob == nil {
				t.Fatalf("Build(%s) returned nil outbound", tc.name)
			}
			if closer, ok := ob.(io.Closer); ok {
				if err := closer.Close(); err != nil {
					t.Fatalf("Build(%s) outbound Close() error: %v", tc.name, err)
				}
			}
		})
	}
}

func TestSingboxBuilder_UnknownType(t *testing.T) {
	b, err := NewSingboxBuilder()
	if err != nil {
		t.Fatalf("NewSingboxBuilder() error: %v", err)
	}
	defer b.Close()

	raw := json.RawMessage(`{"type": "totally-fake-protocol-xyz", "tag": "x"}`)
	_, err = b.Build(raw)
	if err == nil {
		t.Fatal("expected error for unknown outbound type, got nil")
	}
}

func TestSingboxBuilder_InvalidJSON(t *testing.T) {
	b, err := NewSingboxBuilder()
	if err != nil {
		t.Fatalf("NewSingboxBuilder() error: %v", err)
	}
	defer b.Close()

	raw := json.RawMessage(`{invalid`)
	_, err = b.Build(raw)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestSingboxBuilder_ShadowTLSDetour_BuildAndClose(t *testing.T) {
	b, err := NewSingboxBuilder()
	if err != nil {
		t.Fatalf("NewSingboxBuilder() error: %v", err)
	}
	defer b.Close()

	raw := json.RawMessage(`{
		"type": "shadowsocks",
		"tag": "test-ss-stls",
		"server": "127.0.0.1",
		"server_port": 29907,
		"method": "2022-blake3-aes-256-gcm",
		"password": "YjZiNDhjMDg2MWYxOTU3MDA2MTM2YjkzYTg0NzFlMGY=:MzhhOWVhZGt0ODcxNS00M2I1LThkZmMtNTdmMDFmMjQ=",
		"plugin": "shadow-tls",
		"plugin_opts": "host=gateway.icloud.com;password=test-secret;version=3"
	}`)

	ob, err := b.Build(raw)
	if err != nil {
		t.Fatalf("Build(shadowsocks with shadow-tls) error: %v", err)
	}
	if ob == nil {
		t.Fatal("expected non-nil outbound")
	}
	if ob.Type() != "shadowsocks" {
		t.Fatalf("expected outbound type shadowsocks, got %s", ob.Type())
	}
	if ob.Tag() != "test-ss-stls" {
		t.Fatalf("expected outbound tag test-ss-stls, got %s", ob.Tag())
	}

	chained, ok := ob.(*chainedOutbound)
	if !ok {
		t.Fatal("expected *chainedOutbound")
	}
	if chained.detourTag == "" {
		t.Fatal("expected non-empty detourTag")
	}

	// Verify detour outbound exists in outboundMgr
	if _, found := b.outboundMgr.Outbound(chained.detourTag); !found {
		t.Fatalf("expected detour outbound %s in outboundMgr", chained.detourTag)
	}

	// Close outbound and verify detour is cleaned up
	if err := chained.Close(); err != nil {
		t.Fatalf("chained.Close() error: %v", err)
	}

	if _, found := b.outboundMgr.Outbound(chained.detourTag); found {
		t.Fatalf("expected detour outbound %s to be removed from outboundMgr after Close", chained.detourTag)
	}
}

func TestSingboxBuilder_ShadowTLSDetour_Dial(t *testing.T) {
	var connCount atomic.Int32
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			connCount.Add(1)
			_ = conn.Close()
		}
	}()

	tcpAddr := ln.Addr().(*net.TCPAddr)

	b, err := NewSingboxBuilder()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	raw := json.RawMessage(fmt.Sprintf(`{
		"type": "shadowsocks",
		"tag": "test-dial-stls",
		"server": "127.0.0.1",
		"server_port": %d,
		"method": "2022-blake3-aes-256-gcm",
		"password": "YjZiNDhjMDg2MWYxOTU3MDA2MTM2YjkzYTg0NzFlMGY=:MzhhOWVhZGt0ODcxNS00M2I1LThkZmMtNTdmMDFmMjQ=",
		"plugin": "shadow-tls",
		"plugin_opts": "host=gateway.icloud.com;password=test-secret;version=3"
	}`, tcpAddr.Port))

	ob, err := b.Build(raw)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	defer func() {
		if closer, ok := ob.(io.Closer); ok {
			_ = closer.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dest := M.ParseSocksaddr("1.1.1.1:80")
	_, _ = ob.DialContext(ctx, "tcp", dest)

	if connCount.Load() == 0 {
		t.Fatalf("expected outbound DialContext to connect to mock server via detour")
	}
}

func TestSingboxBuilder_ShadowTLSDetour_MissingPassword(t *testing.T) {
	b, err := NewSingboxBuilder()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	raw := json.RawMessage(`{
		"type": "shadowsocks",
		"tag": "test-missing-pass",
		"server": "127.0.0.1",
		"server_port": 8388,
		"method": "2022-blake3-aes-256-gcm",
		"password": "pass",
		"plugin": "shadow-tls",
		"plugin_opts": "host=example.com"
	}`)

	_, err = b.Build(raw)
	if err == nil {
		t.Fatal("expected error when shadow-tls options lack password, got nil")
	}
	if !strings.Contains(err.Error(), "missing password") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestSingboxBuilder_ShadowTLSDetour_MissingHost(t *testing.T) {
	b, err := NewSingboxBuilder()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	raw := json.RawMessage(`{
		"type": "shadowsocks",
		"tag": "test-missing-host",
		"server": "127.0.0.1",
		"server_port": 8388,
		"method": "2022-blake3-aes-256-gcm",
		"password": "pass",
		"plugin": "shadow-tls",
		"plugin_opts": "password=test-secret"
	}`)

	_, err = b.Build(raw)
	if err == nil {
		t.Fatal("expected error when shadow-tls options lack host, got nil")
	}
	if !strings.Contains(err.Error(), "missing host") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// A failed shadowsocks build must not leave its detour registered: the detour
// is created in the outbound manager before the outer outbound.
func TestSingboxBuilder_ShadowTLSDetour_FailedBuildReleasesDetour(t *testing.T) {
	b, err := NewSingboxBuilder()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	raw := json.RawMessage(`{
		"type": "shadowsocks",
		"tag": "test-bad-method",
		"server": "127.0.0.1",
		"server_port": 8388,
		"method": "not-a-real-method",
		"password": "pass",
		"plugin": "shadow-tls",
		"plugin_opts": "host=example.com;password=test-secret"
	}`)

	if _, err := b.Build(raw); err == nil {
		t.Fatal("expected error for unknown shadowsocks method, got nil")
	}
	if n := len(b.outboundMgr.Outbounds()); n != 0 {
		t.Fatalf("expected no detour left registered after failed build, got %d", n)
	}
}

// fakeOutboundManager is a minimal adapter.OutboundManager used to observe
// detour registration and removal without a full sing-box service graph.
// Embedded interface methods are intentionally left unimplemented.
type fakeOutboundManager struct {
	adapter.OutboundManager

	mu         sync.Mutex
	registered map[string]adapter.Outbound
}

func (m *fakeOutboundManager) Outbounds() []adapter.Outbound {
	m.mu.Lock()
	defer m.mu.Unlock()
	outbounds := make([]adapter.Outbound, 0, len(m.registered))
	for _, ob := range m.registered {
		outbounds = append(outbounds, ob)
	}
	return outbounds
}

func (m *fakeOutboundManager) Outbound(tag string) (adapter.Outbound, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ob, loaded := m.registered[tag]
	return ob, loaded
}

func (m *fakeOutboundManager) Remove(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, loaded := m.registered[tag]; !loaded {
		return os.ErrInvalid
	}
	delete(m.registered, tag)
	return nil
}

// countingCloser tracks how many times Close was called.
type countingCloser struct {
	trackCloser
	closes atomic.Int32
}

func (c *countingCloser) Close() error {
	c.closes.Add(1)
	return c.trackCloser.Close()
}

// The sing-box outbound manager only unregisters on Remove while it is not
// started, so chainedOutbound must close the detour itself and do it once.
func TestChainedOutbound_Close_ClosesDetourOnce(t *testing.T) {
	mgr := &fakeOutboundManager{registered: make(map[string]adapter.Outbound)}
	outer := &countingCloser{}
	detour := &countingCloser{}
	chained := &chainedOutbound{
		Outbound:  outer,
		manager:   mgr,
		detour:    detour,
		detourTag: "__resin_stls_test_1",
	}
	mgr.registered[chained.detourTag] = detour

	if err := chained.Close(); err != nil {
		t.Fatalf("chainedOutbound.Close() error: %v", err)
	}
	if !outer.closed.Load() {
		t.Fatal("expected shadowsocks outbound to be closed")
	}
	if !detour.closed.Load() {
		t.Fatal("expected shadowtls detour to be closed")
	}
	if _, loaded := mgr.Outbound(chained.detourTag); loaded {
		t.Fatal("expected detour to be unregistered from the outbound manager")
	}

	if err := chained.Close(); err != nil {
		t.Fatalf("second chainedOutbound.Close() error: %v", err)
	}
	if got := outer.closes.Load(); got != 1 {
		t.Fatalf("expected outer outbound to be closed once, got %d", got)
	}
	if got := detour.closes.Load(); got != 1 {
		t.Fatalf("expected detour outbound to be closed once, got %d", got)
	}
}

func TestStubOutboundBuilder_Build(t *testing.T) {
	ob, err := (&testutil.StubOutboundBuilder{}).Build(nil)
	if err != nil {
		t.Fatalf("StubOutboundBuilder.Build() error: %v", err)
	}
	if ob == nil {
		t.Fatal("expected non-nil outbound")
	}
	if ob.Type() != "stub" {
		t.Fatalf("unexpected outbound type: %s", ob.Type())
	}
}

// ---------------------------------------------------------------------------
// CAS loser close
// ---------------------------------------------------------------------------

// closableBuilder builds closable outbounds that track Close() calls.
type closableBuilder struct {
	mu    sync.Mutex
	built []*trackCloser
}

type trackCloser struct {
	closed atomic.Bool
}

func (c *trackCloser) Close() error {
	c.closed.Store(true)
	return nil
}

func (c *trackCloser) Type() string {
	return "track-closer"
}

func (c *trackCloser) Tag() string {
	return "track-closer"
}

func (c *trackCloser) Network() []string {
	return []string{"tcp", "udp"}
}

func (c *trackCloser) Dependencies() []string {
	return nil
}

func (c *trackCloser) DialContext(context.Context, string, M.Socksaddr) (net.Conn, error) {
	return nil, errors.New("track-closer: dial not supported")
}

func (c *trackCloser) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	return nil, errors.New("track-closer: listen packet not supported")
}

func (b *closableBuilder) Build(_ json.RawMessage) (adapter.Outbound, error) {
	tc := &trackCloser{}
	b.mu.Lock()
	b.built = append(b.built, tc)
	b.mu.Unlock()
	return tc, nil
}

func TestEnsureNodeOutbound_CASLoserClose(t *testing.T) {
	entry := newTestEntry(`{"type":"test"}`)
	pool := &mockPool{}
	pool.addEntry(entry)

	cb := &closableBuilder{}
	mgr := NewOutboundManager(pool, cb)

	// Run many concurrent EnsureNodeOutbound calls. Only the CAS winner's
	// outbound survives; all losers must be closed.
	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			mgr.EnsureNodeOutbound(entry.Hash)
		}()
	}
	wg.Wait()

	if entry.Outbound.Load() == nil {
		t.Fatal("expected outbound to be set")
	}

	cb.mu.Lock()
	total := len(cb.built)
	closedCount := 0
	for _, tc := range cb.built {
		if tc.closed.Load() {
			closedCount++
		}
	}
	cb.mu.Unlock()

	// With N concurrent goroutines, some pass the fast-path nil check before
	// the winner's CAS succeeds. Those losers must all be closed.
	if total > 1 && closedCount != total-1 {
		t.Errorf("expected %d closed outbounds, got %d (total built: %d)", total-1, closedCount, total)
	}
}

// ---------------------------------------------------------------------------
// Remove close
// ---------------------------------------------------------------------------

func TestRemoveNodeOutbound_Closes(t *testing.T) {
	tc := &trackCloser{}
	entry := newTestEntry(`{"type":"test"}`)
	var wrapped adapter.Outbound = tc
	entry.Outbound.Store(&wrapped)

	pool := &mockPool{}
	mgr := NewOutboundManager(pool, &testutil.StubOutboundBuilder{})

	mgr.RemoveNodeOutbound(entry)

	if !tc.closed.Load() {
		t.Fatal("expected outbound to be closed after RemoveNodeOutbound")
	}
	if entry.Outbound.Load() != nil {
		t.Fatal("expected outbound to be nil after RemoveNodeOutbound")
	}
}
