package outbound_test

import (
	"encoding/json"
	"net/netip"
	"regexp"
	"testing"
	"time"

	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/outbound"
	"github.com/Resinat/Resin/internal/platform"
	"github.com/Resinat/Resin/internal/subscription"
	"github.com/Resinat/Resin/internal/testutil"
	"github.com/Resinat/Resin/internal/topology"
)

// TestEndToEnd_NodeEnterRoutableView verifies the full lifecycle:
// node added to pool → EnsureNodeOutbound → outbound set →
// latency recorded → egress IP set → platform filter passes → node in routable view.
func TestEndToEnd_NodeEnterRoutableView(t *testing.T) {
	subMgr := topology.NewSubscriptionManager()

	// Create a subscription with the node.
	rawOpts := json.RawMessage(`{"type":"e2e-test"}`)
	hash := node.HashFromRawOptions(rawOpts)

	// Use a no-op geo lookup (region filters empty → passes).
	geoLookup := func(_ netip.Addr) string { return "US" }

	pool := topology.NewGlobalNodePool(topology.PoolConfig{
		SubLookup:              subMgr.Lookup,
		GeoLookup:              geoLookup,
		MaxLatencyTableEntries: 10,
		MaxConsecutiveFailures: func() int { return 3 },
		LatencyDecayWindow:     func() time.Duration { return 10 * time.Minute },
	})

	// Register a platform with no regex/region filters.
	platCfg := platform.NewPlatform("test-plat-id", "test-plat", []*regexp.Regexp{}, []string{})
	pool.RegisterPlatform(platCfg)

	// Register subscription and set its managed nodes.
	sub := subscription.NewSubscription("sub-1", "Test Sub", "https://example.com/sub", true, false)
	subMgr.Register(sub)
	managedNodes := subscription.NewManagedNodes()
	managedNodes.StoreNode(hash, subscription.ManagedNode{Tags: []string{"tag1"}})
	sub.SwapManagedNodes(managedNodes)

	pool.AddNodeFromSub(hash, rawOpts, "sub-1")

	// At this point, node is in pool but no outbound, no latency, no egress IP.
	// Platform should NOT include it.
	entry, ok := pool.GetEntry(hash)
	if !ok {
		t.Fatal("node not found in pool after AddNodeFromSub")
	}
	if entry.HasOutbound() {
		t.Fatal("expected no outbound before EnsureNodeOutbound")
	}

	// Check platform does NOT contain the node yet.
	plat, ok := pool.GetPlatform("test-plat-id")
	if !ok {
		t.Fatal("platform not found")
	}
	if plat.View().Contains(hash) {
		t.Fatal("node should NOT be in routable view yet (no outbound/latency/egress)")
	}

	// Step 1: Create outbound.
	obMgr := outbound.NewOutboundManager(pool, &testutil.StubOutboundBuilder{})
	obMgr.EnsureNodeOutbound(hash)
	if !entry.HasOutbound() {
		t.Fatal("expected HasOutbound() == true after EnsureNodeOutbound")
	}

	// Step 2: Record latency (simulate a successful probe).
	entry.LatencyTable.Update("cloudflare.com", 50*time.Millisecond, 10*time.Minute)

	// Step 3: Set egress IP.
	ip := netip.MustParseAddr("203.0.113.1")
	entry.SetEgressIP(ip)
	pool.RecordResult(hash, true)

	// Step 4: Trigger platform re-evaluation.
	pool.NotifyNodeDirty(hash)

	// Now platform should include the node.
	if !plat.View().Contains(hash) {
		t.Fatal("node should be in routable view after outbound + latency + egress IP set")
	}

	// Step 5: Remove outbound.
	obMgr.RemoveNodeOutbound(entry)
	pool.NotifyNodeDirty(hash)

	// After removing outbound, platform should exclude the node.
	if plat.View().Contains(hash) {
		t.Fatal("node should NOT be in routable view after outbound removed")
	}
}

func TestEndToEnd_ShadowTLSNodeBuildLifecycle(t *testing.T) {
	subMgr := topology.NewSubscriptionManager()
	pool := topology.NewGlobalNodePool(topology.PoolConfig{
		SubLookup:              subMgr.Lookup,
		MaxLatencyTableEntries: 10,
		MaxConsecutiveFailures: func() int { return 3 },
		LatencyDecayWindow:     func() time.Duration { return 10 * time.Minute },
	})

	builder, err := outbound.NewSingboxBuilder()
	if err != nil {
		t.Fatalf("NewSingboxBuilder() error: %v", err)
	}
	defer builder.Close()

	obMgr := outbound.NewOutboundManager(pool, builder)

	yamlData := []byte(`
proxies:
  - name: "🇸🇬 SG 07"
    type: ss
    server: tqt-ss.ftnode369.com
    port: 29907
    cipher: 2022-blake3-aes-256-gcm
    password: YjZiNDhjMDg2MWYxOTU3MDA2MTM2YjkzYTg0NzFlMGY=:MzhhOWVhZGt0ODcxNS00M2I1LThkZmMtNTdmMDFmMjQ=
    plugin: shadow-tls
    plugin-opts:
      host: gateway.icloud.com
      password: test-shadowtls-password
      version: 3
`)

	nodes, err := subscription.ParseGeneralSubscription(yamlData)
	if err != nil {
		t.Fatalf("ParseGeneralSubscription: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	hash := node.HashFromRawOptions(nodes[0].RawOptions)
	pool.AddNodeFromSub(hash, nodes[0].RawOptions, "sub-1")

	entry, ok := pool.GetEntry(hash)
	if !ok {
		t.Fatal("expected node in pool")
	}

	obMgr.EnsureNodeOutbound(hash)

	if !entry.HasOutbound() {
		t.Fatalf("expected outbound to be created, last error: %s", entry.GetLastError())
	}
	if entry.GetLastError() != "" {
		t.Fatalf("unexpected last error: %s", entry.GetLastError())
	}

	obPtr := entry.Outbound.Load()
	if obPtr == nil || *obPtr == nil {
		t.Fatal("expected non-nil outbound")
	}
	ob := *obPtr
	if ob.Type() != "shadowsocks" {
		t.Fatalf("expected type shadowsocks, got %s", ob.Type())
	}

	// Remove and verify clean cleanup
	obMgr.RemoveNodeOutbound(entry)
	if entry.HasOutbound() {
		t.Fatal("expected outbound to be nil after remove")
	}
}
