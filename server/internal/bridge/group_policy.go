package bridge

import (
	"encoding/json"
	"errors"
	"strings"
)

// GroupPolicy controls which WhatsApp groups are visible to the bridge.
// Its fields are private so a policy cannot change after construction.
type GroupPolicy struct {
	mode      string
	allowlist map[string]struct{}
}

const (
	GroupModePinned = "pinned"
	GroupModeAll    = "all"
)

func NewGroupPolicy(mode string, allowlist []string) (GroupPolicy, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = GroupModePinned
	}
	if mode != GroupModePinned && mode != GroupModeAll {
		return GroupPolicy{}, errors.New("GROUP_MODE must be pinned or all")
	}
	groups := make(map[string]struct{}, len(allowlist))
	for _, name := range allowlist {
		normalized := normalizeGroupName(name)
		if normalized == "" {
			return GroupPolicy{}, errors.New("GROUP_ALLOWLIST entries must not be empty")
		}
		groups[normalized] = struct{}{}
	}
	return GroupPolicy{mode: mode, allowlist: groups}, nil
}

func GroupPolicyFromJSON(mode, rawAllowlist string) (GroupPolicy, error) {
	var allowlist []string
	if normalized := strings.TrimSpace(rawAllowlist); normalized != "" {
		if normalized == "null" {
			return GroupPolicy{}, errors.New("GROUP_ALLOWLIST must be a JSON array of group names")
		}
		if err := json.Unmarshal([]byte(normalized), &allowlist); err != nil {
			return GroupPolicy{}, errors.New("GROUP_ALLOWLIST must be a JSON array of group names")
		}
	}
	return NewGroupPolicy(mode, allowlist)
}

func DefaultGroupPolicy() GroupPolicy {
	policy, err := NewGroupPolicy(GroupModePinned, nil)
	if err != nil {
		panic(err)
	}
	return policy
}

func (p GroupPolicy) Includes(name string, pinned bool) bool {
	if p.mode == GroupModeAll || pinned {
		return true
	}
	_, allowed := p.allowlist[normalizeGroupName(name)]
	return allowed
}

func (p GroupPolicy) Mode() string { return p.mode }

func (p GroupPolicy) Allows(name string) bool {
	_, allowed := p.allowlist[normalizeGroupName(name)]
	return allowed
}

func normalizeGroupName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}
