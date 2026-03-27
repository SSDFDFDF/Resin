package platform

import "strings"

// StickyLeaseMode controls how account stickiness expires.
type StickyLeaseMode string

const (
	// StickyLeaseModeTTL keeps the existing TTL-based sticky lease behavior.
	StickyLeaseModeTTL StickyLeaseMode = "TTL"
	// StickyLeaseModeManual keeps leases until explicit cleanup or configured auto-clean.
	StickyLeaseModeManual StickyLeaseMode = "MANUAL"
)

func (m StickyLeaseMode) IsValid() bool {
	switch m {
	case StickyLeaseModeTTL, StickyLeaseModeManual:
		return true
	default:
		return false
	}
}

func NormalizeStickyLeaseMode(raw string) StickyLeaseMode {
	mode := StickyLeaseMode(strings.TrimSpace(raw))
	if !mode.IsValid() {
		return StickyLeaseModeTTL
	}
	return mode
}
