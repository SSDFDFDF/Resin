package platform

import "strings"

// ManualUnavailableAction controls how MANUAL sticky leases react when the
// anchored egress IP has no remaining healthy nodes.
type ManualUnavailableAction string

const (
	// ManualUnavailableActionHold keeps the lease and waits for the anchored IP.
	ManualUnavailableActionHold ManualUnavailableAction = "HOLD"
	// ManualUnavailableActionAutoClean removes the lease after grace and re-routes.
	ManualUnavailableActionAutoClean ManualUnavailableAction = "AUTO_CLEAN"
)

func (a ManualUnavailableAction) IsValid() bool {
	switch a {
	case ManualUnavailableActionHold, ManualUnavailableActionAutoClean:
		return true
	default:
		return false
	}
}

func ParseManualUnavailableAction(raw string) (ManualUnavailableAction, bool) {
	action := ManualUnavailableAction(strings.TrimSpace(raw))
	if action == "" {
		return ManualUnavailableActionHold, true
	}
	if !action.IsValid() {
		return "", false
	}
	return action, true
}

func NormalizeManualUnavailableAction(raw string) ManualUnavailableAction {
	action, ok := ParseManualUnavailableAction(raw)
	if !ok {
		return ManualUnavailableActionHold
	}
	return action
}
