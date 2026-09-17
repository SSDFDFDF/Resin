package outbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/endpoint"
	"github.com/sagernet/sing-box/adapter/inbound"
	sbOutbound "github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
	sJson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"
)

// OutboundBuilder creates outbound instances from raw node options.
type OutboundBuilder interface {
	Build(rawOptions json.RawMessage) (adapter.Outbound, error)
}

// ---------------------------------------------------------------------------
// SingboxBuilder — creates real sing-box adapter.Outbound instances.
// ---------------------------------------------------------------------------

// SingboxBuilder builds real sing-box outbound instances from raw JSON options.
// It holds a fully-wired context with DNS services so that domain-based
// outbound servers can be resolved.
type SingboxBuilder struct {
	registry *sbOutbound.Registry
	// outboundMgr is sing-box's own manager, used solely as the detour registry
	// that DetourDialer resolves detour tags against. It is never started, so
	// its lifecycle helpers (Close/Remove) do not close outbounds; detour
	// instances are owned and closed by their chainedOutbound wrapper.
	outboundMgr         adapter.OutboundManager
	ctx                 context.Context
	logFactory          log.Factory
	dnsTransportManager *dns.TransportManager
	dnsRouter           *dns.Router
	nextID              atomic.Uint64
}

// NewSingboxBuilder creates a SingboxBuilder with a complete sing-box service
// graph (registries + DNS). The caller must call Close() when done.
func NewSingboxBuilder() (*SingboxBuilder, error) {
	ctx := context.Background()
	ctx = include.Context(ctx) // inject protocol registries

	logFactory := log.NewNOPFactory()
	logger := logFactory.NewLogger("resin-outbound")

	// --- Service graph (same order as Demos/simple-proxy/main.go) -----------

	// Endpoint Manager
	endpointMgr := endpoint.NewManager(logger, service.FromContext[adapter.EndpointRegistry](ctx))
	service.MustRegister[adapter.EndpointManager](ctx, endpointMgr)

	// Inbound Manager (required dependency even though unused)
	inboundMgr := inbound.NewManager(logger, service.FromContext[adapter.InboundRegistry](ctx), endpointMgr)
	service.MustRegister[adapter.InboundManager](ctx, inboundMgr)

	// Outbound Manager (sing-box's own manager, for detour resolution)
	outboundMgr := sbOutbound.NewManager(logger, service.FromContext[adapter.OutboundRegistry](ctx), endpointMgr, "")
	service.MustRegister[adapter.OutboundManager](ctx, outboundMgr)

	// DNS Transport Manager
	dnsTransportMgr := dns.NewTransportManager(logger, service.FromContext[adapter.DNSTransportRegistry](ctx), outboundMgr, "")
	service.MustRegister[adapter.DNSTransportManager](ctx, dnsTransportMgr)

	// DNS Router
	dnsRouter := dns.NewRouter(ctx, logFactory, option.DNSOptions{})
	service.MustRegister[adapter.DNSRouter](ctx, dnsRouter)

	// Register local DNS transport
	if err := dnsTransportMgr.Create(ctx, logger, "local", "local", &option.LocalDNSServerOptions{}); err != nil {
		return nil, fmt.Errorf("singbox builder: create local DNS transport: %w", err)
	}

	// Start DNS Transport Manager lifecycle
	if err := dnsTransportMgr.Start(adapter.StartStateInitialize); err != nil {
		return nil, fmt.Errorf("singbox builder: initialize DNS transport manager: %w", err)
	}
	if err := dnsTransportMgr.Start(adapter.StartStateStart); err != nil {
		_ = dnsTransportMgr.Close()
		return nil, fmt.Errorf("singbox builder: start DNS transport manager: %w", err)
	}

	// Start DNS Router lifecycle
	if err := dnsRouter.Initialize(nil); err != nil {
		_ = dnsTransportMgr.Close()
		return nil, fmt.Errorf("singbox builder: initialize DNS router: %w", err)
	}
	if err := dnsRouter.Start(adapter.StartStateStart); err != nil {
		_ = dnsRouter.Close()
		_ = dnsTransportMgr.Close()
		return nil, fmt.Errorf("singbox builder: start DNS router: %w", err)
	}

	registry := service.FromContext[adapter.OutboundRegistry](ctx).(*sbOutbound.Registry)

	return &SingboxBuilder{
		registry:            registry,
		outboundMgr:         outboundMgr,
		ctx:                 ctx,
		logFactory:          logFactory,
		dnsTransportManager: dnsTransportMgr,
		dnsRouter:           dnsRouter,
	}, nil
}

// Build parses rawOptions (a complete sing-box outbound JSON object with
// type/tag fields) into a real adapter.Outbound and runs it through the
// lifecycle stages.
func (b *SingboxBuilder) Build(rawOptions json.RawMessage) (adapter.Outbound, error) {
	// 1. Parse via official option.Outbound path (strips type/tag, creates
	//    typed options via OutboundOptionsRegistry + badjson.UnmarshallExcluded).
	var outboundConfig option.Outbound
	if err := sJson.UnmarshalContext(b.ctx, rawOptions, &outboundConfig); err != nil {
		return nil, fmt.Errorf("parse outbound options: %w", err)
	}

	// Intercept shadowsocks with shadow-tls plugin to build a native chained detour.
	if outboundConfig.Type == "shadowsocks" {
		if ssOptions, ok := outboundConfig.Options.(*option.ShadowsocksOutboundOptions); ok && isShadowTLSPlugin(ssOptions.Plugin) {
			return b.buildShadowTLSChained(outboundConfig.Tag, ssOptions)
		}
	}

	// 2. Create the outbound instance via the registry.
	logger := b.logFactory.NewLogger("outbound/" + outboundConfig.Type)
	ob, err := b.registry.CreateOutbound(
		b.ctx,
		nil, // router — not needed for simple dialing
		logger,
		outboundConfig.Tag,
		outboundConfig.Type,
		outboundConfig.Options,
	)
	if err != nil {
		return nil, fmt.Errorf("create outbound [%s]: %w", outboundConfig.Type, err)
	}

	// 3. Run lifecycle start stages. On failure, close and return error.
	for _, stage := range adapter.ListStartStages {
		if err := adapter.LegacyStart(ob, stage); err != nil {
			_ = common.Close(ob)
			return nil, fmt.Errorf("outbound start %s [%s]: %w", stage, outboundConfig.Type, err)
		}
	}

	return ob, nil
}

// Close shuts down the builder's internal DNS services.
//
// The outbound manager is deliberately not closed here: it is never started,
// and sing-box's Manager.Close() is a no-op in that state. Detour instances
// live in chainedOutbound and are released together with their node outbound.
func (b *SingboxBuilder) Close() error {
	var errs []error
	if b.dnsRouter != nil {
		errs = append(errs, b.dnsRouter.Close())
	}
	if b.dnsTransportManager != nil {
		errs = append(errs, b.dnsTransportManager.Close())
	}
	return errors.Join(errs...)
}

func isShadowTLSPlugin(plugin string) bool {
	p := strings.ToLower(strings.TrimSpace(plugin))
	return p == "shadow-tls" || p == "shadowtls"
}

type shadowTLSOptions struct {
	host        string
	password    string
	version     int
	fingerprint string
	insecure    bool
}

func parseShadowTLSPluginOptions(rawOpts string) shadowTLSOptions {
	opts := shadowTLSOptions{
		version: 3,
	}
	for _, part := range strings.Split(rawOpts, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		if !ok {
			if k == "insecure" || k == "allowinsecure" || k == "skip-cert-verify" {
				opts.insecure = true
			}
			continue
		}
		switch k {
		case "host", "sni", "server_name", "servername", "peer":
			opts.host = v
		case "password", "secret", "token", "auth":
			opts.password = v
		case "version", "ver", "v":
			if ver, err := strconv.Atoi(v); err == nil && ver > 0 {
				if ver > 3 {
					ver = 3
				}
				opts.version = ver
			}
		case "fingerprint", "client-fingerprint", "client_fingerprint", "fp":
			opts.fingerprint = v
		case "insecure", "allowinsecure", "skip-cert-verify":
			opts.insecure = (v == "1" || strings.EqualFold(v, "true"))
		}
	}
	return opts
}

func (b *SingboxBuilder) buildShadowTLSChained(tag string, ssOptions *option.ShadowsocksOutboundOptions) (adapter.Outbound, error) {
	stlsOpts := parseShadowTLSPluginOptions(ssOptions.PluginOptions)
	if stlsOpts.password == "" {
		return nil, fmt.Errorf("shadow-tls plugin missing password in options: %q", ssOptions.PluginOptions)
	}
	// The TLS server name is the shadow-tls front domain; it cannot be inferred
	// from the node address (usually an IP), so a missing host is a config error
	// that would otherwise only surface as an unattributable dial failure.
	if stlsOpts.host == "" {
		return nil, fmt.Errorf("shadow-tls plugin missing host in options: %q", ssOptions.PluginOptions)
	}
	if stlsOpts.version < 1 || stlsOpts.version > 3 {
		stlsOpts.version = 3
	}

	tlsOpts := &option.OutboundTLSOptions{
		Enabled:    true,
		ServerName: stlsOpts.host,
		Insecure:   stlsOpts.insecure,
	}
	if stlsOpts.fingerprint != "" {
		tlsOpts.UTLS = &option.OutboundUTLSOptions{
			Enabled:     true,
			Fingerprint: stlsOpts.fingerprint,
		}
	}

	stlsOutboundOptions := option.ShadowTLSOutboundOptions{
		ServerOptions: option.ServerOptions{
			Server:     ssOptions.Server,
			ServerPort: ssOptions.ServerPort,
		},
		Version:  stlsOpts.version,
		Password: stlsOpts.password,
		OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{
			TLS: tlsOpts,
		},
	}

	detourTag := fmt.Sprintf("__resin_stls_%d", b.nextID.Add(1))
	stlsLogger := b.logFactory.NewLogger("outbound/shadowtls")
	if err := b.outboundMgr.Create(b.ctx, nil, stlsLogger, detourTag, "shadowtls", &stlsOutboundOptions); err != nil {
		return nil, fmt.Errorf("create shadowtls detour outbound: %w", err)
	}

	stlsOb, ok := b.outboundMgr.Outbound(detourTag)
	if !ok {
		_ = releaseDetour(b.outboundMgr, detourTag, nil)
		return nil, fmt.Errorf("shadowtls detour outbound %q not found in manager", detourTag)
	}
	for _, stage := range adapter.ListStartStages {
		if err := adapter.LegacyStart(stlsOb, stage); err != nil {
			_ = releaseDetour(b.outboundMgr, detourTag, stlsOb)
			return nil, fmt.Errorf("start shadowtls detour stage %s: %w", stage, err)
		}
	}

	ssOptionsCopy := *ssOptions
	ssOptionsCopy.Plugin = ""
	ssOptionsCopy.PluginOptions = ""
	ssOptionsCopy.Detour = detourTag

	ssLogger := b.logFactory.NewLogger("outbound/shadowsocks")
	ssOb, err := b.registry.CreateOutbound(b.ctx, nil, ssLogger, tag, "shadowsocks", &ssOptionsCopy)
	if err != nil {
		_ = releaseDetour(b.outboundMgr, detourTag, stlsOb)
		return nil, fmt.Errorf("create shadowsocks outbound with shadowtls detour: %w", err)
	}

	for _, stage := range adapter.ListStartStages {
		if err := adapter.LegacyStart(ssOb, stage); err != nil {
			_ = common.Close(ssOb)
			_ = releaseDetour(b.outboundMgr, detourTag, stlsOb)
			return nil, fmt.Errorf("outbound start %s [shadowsocks]: %w", stage, err)
		}
	}

	return &chainedOutbound{
		Outbound:  ssOb,
		manager:   b.outboundMgr,
		detour:    stlsOb,
		detourTag: detourTag,
	}, nil
}

// releaseDetour unregisters a detour from the sing-box outbound manager and
// closes it. The manager's Remove() alone is not enough: while the manager is
// not started it only unregisters, leaving the instance unclosed. detour may be
// nil when the instance could not be resolved; common.Close skips it.
func releaseDetour(manager adapter.OutboundManager, detourTag string, detour adapter.Outbound) error {
	var errs []error
	if manager != nil && detourTag != "" {
		errs = append(errs, manager.Remove(detourTag))
	}
	return errors.Join(append(errs, common.Close(detour))...)
}

// chainedOutbound pairs a node's shadowsocks outbound with the shadowtls
// outbound it dials through. The embedded shadowsocks outbound stays the node's
// public identity (Type/Tag/DialContext); the detour is an internal detail
// whose lifetime is bound to this wrapper.
type chainedOutbound struct {
	adapter.Outbound
	manager   adapter.OutboundManager
	detour    adapter.Outbound
	detourTag string
	closed    atomic.Bool
}

func (c *chainedOutbound) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	// Close the outer outbound first, then release the detour it dials through.
	// common.Close is used because adapter.Outbound does not require Close
	// (shadowtls does not implement it); it is nil-safe and skips values without
	// one.
	return errors.Join(common.Close(c.Outbound), releaseDetour(c.manager, c.detourTag, c.detour))
}
