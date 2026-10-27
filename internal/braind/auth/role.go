// Package auth implements OpenID Connect sign-in, server-side sessions, and the
// single role policy that browser and JSON surfaces share (spec 002, ADR-0004 and
// ADR-0005). Provider interaction follows the stdlib-only pattern proven by
// tools/oidcverify. Production code never imports auth/authtest, the simulated
// provider used by tests.
package auth

// Role is one of the three access levels or no access at all (data-model.md).
type Role string

const (
	RoleAdministrator Role = "administrator"
	RoleUser          Role = "user"
	RoleViewer        Role = "viewer"
	RoleNone          Role = ""
)

// RoleConfig carries the distinct group names mapped to each role. Config
// validation guarantees the three values are mutually distinct.
type RoleConfig struct {
	AdminGroup  string
	UserGroup   string
	ViewerGroup string
}

// ResolveRole maps group membership to one effective role. Matching is exact and
// case-sensitive; partial membership never counts. When more than one configured
// group matches, administrator wins, then user, then viewer (FR-010). An empty or
// absent group list resolves to RoleNone, which denies protected access (FR-005).
func ResolveRole(groups []string, config RoleConfig) Role {
	membership := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		membership[group] = struct{}{}
	}
	if config.AdminGroup != "" {
		if _, ok := membership[config.AdminGroup]; ok {
			return RoleAdministrator
		}
	}
	if config.UserGroup != "" {
		if _, ok := membership[config.UserGroup]; ok {
			return RoleUser
		}
	}
	if config.ViewerGroup != "" {
		if _, ok := membership[config.ViewerGroup]; ok {
			return RoleViewer
		}
	}
	return RoleNone
}

// GroupsFromClaim extracts group names from a decoded claim value. It accepts a
// JSON array of strings or a single string (the shape some providers emit) and
// reports false for absent, mixed-type, or non-string values so callers deny
// access rather than guessing (US2-AC3).
func GroupsFromClaim(value any) ([]string, bool) {
	switch claim := value.(type) {
	case []string:
		return append([]string(nil), claim...), true
	case []any:
		groups := make([]string, 0, len(claim))
		for _, item := range claim {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			groups = append(groups, text)
		}
		return groups, true
	case string:
		if claim == "" {
			return nil, false
		}
		return []string{claim}, true
	default:
		return nil, false
	}
}

// Meets reports whether role satisfies the requested minimum. RoleNone satisfies
// nothing; hierarchy order is viewer < user < administrator.
func (r Role) Meets(minimum Role) bool {
	if minimum == RoleNone {
		return true
	}
	switch minimum {
	case RoleViewer:
		return r == RoleViewer || r == RoleUser || r == RoleAdministrator
	case RoleUser:
		return r == RoleUser || r == RoleAdministrator
	case RoleAdministrator:
		return r == RoleAdministrator
	default:
		return false
	}
}
