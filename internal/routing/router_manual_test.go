package routing

import (
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/Resinat/Resin/internal/model"
	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/platform"
)

func notifyRouterTestNodeDirty(t *testing.T, plat *platform.Platform, pool *routerTestPool, h node.Hash) {
	t.Helper()
	plat.NotifyDirty(
		h,
		pool.GetEntry,
		func(_ string, _ node.Hash) (string, bool, []string, bool) { return "", true, nil, true },
		func(_ netip.Addr) string { return "" },
	)
}

func TestRouteRequest_ManualStickyHoldKeepsLeaseUntilSameIPReturns(t *testing.T) {
	pool := newRouterTestPool()
	plat := platform.NewPlatform("plat-manual-hold", "Plat-Manual-Hold", nil, nil)
	plat.StickyLeaseMode = string(platform.StickyLeaseModeManual)
	plat.ManualUnavailableAction = string(platform.ManualUnavailableActionHold)
	pool.addPlatform(plat)

	currentHash, currentEntry := newRoutableEntry(t, `{"id":"manual-hold-current"}`, "198.51.100.10")
	pool.addEntry(currentHash, currentEntry)
	pool.rebuildPlatformView(plat)

	router := newTestRouter(pool, nil)
	res1, err := router.RouteRequest(plat.Name, "acct-hold", "https://example.com")
	if err != nil {
		t.Fatalf("initial route: %v", err)
	}
	if res1.EgressIP != netip.MustParseAddr("198.51.100.10") {
		t.Fatalf("initial egress ip: got=%s want=%s", res1.EgressIP, "198.51.100.10")
	}

	otherHash, otherEntry := newRoutableEntry(t, `{"id":"manual-hold-other"}`, "198.51.100.11")
	pool.addEntry(otherHash, otherEntry)
	pool.rebuildPlatformView(plat)

	currentEntry.CircuitOpenSince.Store(time.Now().UnixNano())
	notifyRouterTestNodeDirty(t, plat, pool, currentHash)

	_, err = router.RouteRequest(plat.Name, "acct-hold", "https://example.com")
	if !errors.Is(err, ErrNoAvailableNodes) {
		t.Fatalf("expected ErrNoAvailableNodes while anchor IP unavailable, got %v", err)
	}

	lease := router.ReadLease(model.LeaseKey{PlatformID: plat.ID, Account: "acct-hold"})
	if lease == nil {
		t.Fatal("expected manual hold lease to remain present")
	}
	if lease.EgressIP != "198.51.100.10" {
		t.Fatalf("held lease egress_ip: got %s want %s", lease.EgressIP, "198.51.100.10")
	}
	if lease.UnavailableSinceNs <= 0 {
		t.Fatalf("expected unavailable_since_ns to be recorded, got %d", lease.UnavailableSinceNs)
	}

	recoveryHash, recoveryEntry := newRoutableEntry(t, `{"id":"manual-hold-recovery"}`, "198.51.100.10")
	pool.addEntry(recoveryHash, recoveryEntry)
	pool.rebuildPlatformView(plat)

	res2, err := router.RouteRequest(plat.Name, "acct-hold", "https://example.com")
	if err != nil {
		t.Fatalf("route after same-ip recovery: %v", err)
	}
	if res2.EgressIP != netip.MustParseAddr("198.51.100.10") {
		t.Fatalf("recovered egress ip: got=%s want=%s", res2.EgressIP, "198.51.100.10")
	}
	if res2.NodeHash != recoveryHash {
		t.Fatalf("expected same-ip rotation to recovered node: got=%s want=%s", res2.NodeHash.Hex(), recoveryHash.Hex())
	}

	lease = router.ReadLease(model.LeaseKey{PlatformID: plat.ID, Account: "acct-hold"})
	if lease == nil {
		t.Fatal("expected lease after recovery")
	}
	if lease.UnavailableSinceNs != 0 {
		t.Fatalf("expected unavailable_since_ns reset after recovery, got %d", lease.UnavailableSinceNs)
	}
}

func TestRouteRequest_ManualStickyAutoCleanReassignsImmediately(t *testing.T) {
	pool := newRouterTestPool()
	plat := platform.NewPlatform("plat-manual-auto", "Plat-Manual-Auto", nil, nil)
	plat.StickyLeaseMode = string(platform.StickyLeaseModeManual)
	plat.ManualUnavailableAction = string(platform.ManualUnavailableActionAutoClean)
	pool.addPlatform(plat)

	currentHash, currentEntry := newRoutableEntry(t, `{"id":"manual-auto-current"}`, "203.0.113.10")
	pool.addEntry(currentHash, currentEntry)
	pool.rebuildPlatformView(plat)

	var events []LeaseEvent
	router := newTestRouter(pool, func(e LeaseEvent) {
		events = append(events, e)
	})

	if _, err := router.RouteRequest(plat.Name, "acct-auto", "https://example.com"); err != nil {
		t.Fatalf("initial route: %v", err)
	}

	replacementHash, replacementEntry := newRoutableEntry(t, `{"id":"manual-auto-replacement"}`, "203.0.113.11")
	pool.addEntry(replacementHash, replacementEntry)
	pool.rebuildPlatformView(plat)

	currentEntry.CircuitOpenSince.Store(time.Now().UnixNano())
	notifyRouterTestNodeDirty(t, plat, pool, currentHash)

	res2, err := router.RouteRequest(plat.Name, "acct-auto", "https://example.com")
	if err != nil {
		t.Fatalf("auto-clean reroute: %v", err)
	}
	if !res2.LeaseCreated {
		t.Fatal("auto-clean reroute should create a fresh lease")
	}
	if res2.NodeHash != replacementHash {
		t.Fatalf("replacement node: got=%s want=%s", res2.NodeHash.Hex(), replacementHash.Hex())
	}
	if res2.EgressIP != netip.MustParseAddr("203.0.113.11") {
		t.Fatalf("replacement egress ip: got=%s want=%s", res2.EgressIP, "203.0.113.11")
	}

	lease := router.ReadLease(model.LeaseKey{PlatformID: plat.ID, Account: "acct-auto"})
	if lease == nil {
		t.Fatal("expected lease after auto-clean reroute")
	}
	if lease.EgressIP != "203.0.113.11" {
		t.Fatalf("stored replacement egress ip: got=%s want=%s", lease.EgressIP, "203.0.113.11")
	}
	if lease.UnavailableSinceNs != 0 {
		t.Fatalf("expected unavailable_since_ns cleared on replacement, got %d", lease.UnavailableSinceNs)
	}

	foundRemove := false
	foundCreate := false
	for _, e := range events {
		if e.Type == LeaseRemove && e.Account == "acct-auto" && e.EgressIP == netip.MustParseAddr("203.0.113.10") {
			foundRemove = true
		}
		if e.Type == LeaseCreate && e.Account == "acct-auto" && e.EgressIP == netip.MustParseAddr("203.0.113.11") {
			foundCreate = true
		}
	}
	if !foundRemove || !foundCreate {
		t.Fatalf("expected remove+create events during auto-clean, got %+v", events)
	}
}

func TestRouteRequest_ManualStickyAutoCleanHonorsGraceBeforeReassign(t *testing.T) {
	pool := newRouterTestPool()
	plat := platform.NewPlatform("plat-manual-grace", "Plat-Manual-Grace", nil, nil)
	plat.StickyLeaseMode = string(platform.StickyLeaseModeManual)
	plat.ManualUnavailableAction = string(platform.ManualUnavailableActionAutoClean)
	plat.ManualUnavailableGraceNs = int64(time.Minute)
	pool.addPlatform(plat)

	currentHash, currentEntry := newRoutableEntry(t, `{"id":"manual-grace-current"}`, "203.0.113.20")
	pool.addEntry(currentHash, currentEntry)
	pool.rebuildPlatformView(plat)

	router := newTestRouter(pool, nil)
	if _, err := router.RouteRequest(plat.Name, "acct-grace", "https://example.com"); err != nil {
		t.Fatalf("initial route: %v", err)
	}

	replacementHash, replacementEntry := newRoutableEntry(t, `{"id":"manual-grace-replacement"}`, "203.0.113.21")
	pool.addEntry(replacementHash, replacementEntry)
	pool.rebuildPlatformView(plat)

	currentEntry.CircuitOpenSince.Store(time.Now().UnixNano())
	notifyRouterTestNodeDirty(t, plat, pool, currentHash)

	_, err := router.RouteRequest(plat.Name, "acct-grace", "https://example.com")
	if !errors.Is(err, ErrNoAvailableNodes) {
		t.Fatalf("expected ErrNoAvailableNodes before grace elapses, got %v", err)
	}

	state, ok := router.states.Load(plat.ID)
	if !ok {
		t.Fatal("expected routing state for platform")
	}
	lease, ok := state.Leases.GetLease("acct-grace")
	if !ok {
		t.Fatal("expected stored lease before grace elapses")
	}
	if lease.UnavailableSinceNs <= 0 {
		t.Fatalf("expected unavailable_since_ns to be set, got %d", lease.UnavailableSinceNs)
	}

	lease.UnavailableSinceNs = time.Now().Add(-2 * time.Minute).UnixNano()
	state.Leases.CreateLease("acct-grace", lease)

	res2, err := router.RouteRequest(plat.Name, "acct-grace", "https://example.com")
	if err != nil {
		t.Fatalf("reroute after grace: %v", err)
	}
	if res2.NodeHash != replacementHash {
		t.Fatalf("replacement after grace: got=%s want=%s", res2.NodeHash.Hex(), replacementHash.Hex())
	}
	if res2.EgressIP != netip.MustParseAddr("203.0.113.21") {
		t.Fatalf("replacement egress ip after grace: got=%s want=%s", res2.EgressIP, "203.0.113.21")
	}
}

func TestLeaseCleaner_IgnoresManualLeasesWithoutExpiry(t *testing.T) {
	pool := newRouterTestPool()
	plat := platform.NewPlatform("plat-manual-cleaner", "Plat-Manual-Cleaner", nil, nil)
	plat.StickyLeaseMode = string(platform.StickyLeaseModeManual)
	pool.addPlatform(plat)

	router := newTestRouter(pool, nil)
	state := router.ensurePlatformState(plat.ID)
	state.Leases.CreateLease("acct-manual", Lease{
		NodeHash:           node.HashFromRawOptions([]byte(`{"id":"manual-cleaner"}`)),
		EgressIP:           netip.MustParseAddr("198.51.100.50"),
		CreatedAtNs:        time.Now().Add(-time.Hour).UnixNano(),
		ExpiryNs:           0,
		LastAccessedNs:     time.Now().Add(-time.Minute).UnixNano(),
		UnavailableSinceNs: 0,
	})

	cleaner := NewLeaseCleaner(router)
	cleaner.sweepPlatformState(plat.ID, state, time.Now().UnixNano())

	if _, ok := state.Leases.GetLease("acct-manual"); !ok {
		t.Fatal("manual lease should not be removed by cleaner when expiry is disabled")
	}
}
