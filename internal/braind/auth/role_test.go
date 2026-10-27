package auth

import "testing"

func TestResolveRoleMapsMembership(t *testing.T) {
	config := RoleConfig{AdminGroup: "access-brain-admin", UserGroup: "access-brain-user", ViewerGroup: "access-brain-viewer"}
	tests := []struct {
		name   string
		groups []string
		want   Role
	}{
		{name: "administrator", groups: []string{"access-brain-admin"}, want: RoleAdministrator},
		{name: "user", groups: []string{"access-brain-user"}, want: RoleUser},
		{name: "viewer", groups: []string{"access-brain-viewer"}, want: RoleViewer},
		{name: "unknown groups deny", groups: []string{"other-team"}, want: RoleNone},
		{name: "no groups", groups: nil, want: RoleNone},
		{name: "empty group name does not match", groups: []string{""}, want: RoleNone},
		{name: "unconfigured empty group never grants", groups: []string{""}, want: RoleNone},
		{name: "partial match is not membership", groups: []string{"access-brain-us"}, want: RoleNone},
		{name: "case difference is not membership", groups: []string{"Access-Brain-Admin"}, want: RoleNone},
		{name: "duplicated membership still resolves", groups: []string{"access-brain-user", "access-brain-user"}, want: RoleUser},
		{name: "user plus viewer prefers user", groups: []string{"access-brain-user", "access-brain-viewer"}, want: RoleUser},
		{name: "administrator plus viewer prefers administrator", groups: []string{"access-brain-viewer", "access-brain-admin"}, want: RoleAdministrator},
		{name: "all three prefers administrator", groups: []string{"access-brain-viewer", "access-brain-user", "access-brain-admin"}, want: RoleAdministrator},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ResolveRole(test.groups, config); got != test.want {
				t.Fatalf("ResolveRole(%q) = %q, want %q", test.groups, got, test.want)
			}
		})
	}
}

func TestResolveRoleIgnoresUnconfiguredGroups(t *testing.T) {
	// A role group that is unset must not turn an empty claim value into access.
	if got := ResolveRole([]string{""}, RoleConfig{AdminGroup: "admin"}); got != RoleNone {
		t.Fatalf("unset groups granted %q", got)
	}
}

func TestGroupsFromClaim(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		wantGroups []string
		wantOK     bool
	}{
		{name: "string array", value: []any{"access-brain-user"}, wantGroups: []string{"access-brain-user"}, wantOK: true},
		{name: "native string array", value: []string{"a", "b"}, wantGroups: []string{"a", "b"}, wantOK: true},
		{name: "single string provider shape", value: "access-brain-viewer", wantGroups: []string{"access-brain-viewer"}, wantOK: true},
		{name: "absent claim", value: nil, wantOK: false},
		{name: "empty string", value: "", wantOK: false},
		{name: "non-string element", value: []any{"ok", 7}, wantOK: false},
		{name: "number", value: 12, wantOK: false},
		{name: "object", value: map[string]any{"x": 1}, wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			groups, ok := GroupsFromClaim(test.value)
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}
			if !test.wantOK {
				return
			}
			if len(groups) != len(test.wantGroups) {
				t.Fatalf("groups = %q, want %q", groups, test.wantGroups)
			}
			for index := range groups {
				if groups[index] != test.wantGroups[index] {
					t.Fatalf("groups[%d] = %q, want %q", index, groups[index], test.wantGroups[index])
				}
			}
		})
	}
}

func TestRoleMeets(t *testing.T) {
	tests := []struct {
		role     Role
		minimum  Role
		expected bool
	}{
		{role: RoleAdministrator, minimum: RoleNone, expected: true},
		{role: RoleViewer, minimum: RoleNone, expected: true},
		{role: RoleNone, minimum: RoleNone, expected: true},
		{role: RoleNone, minimum: RoleViewer, expected: false},
		{role: RoleViewer, minimum: RoleViewer, expected: true},
		{role: RoleViewer, minimum: RoleUser, expected: false},
		{role: RoleUser, minimum: RoleViewer, expected: true},
		{role: RoleUser, minimum: RoleAdministrator, expected: false},
		{role: RoleAdministrator, minimum: RoleAdministrator, expected: true},
		{role: RoleAdministrator, minimum: RoleUser, expected: true},
	}
	for _, test := range tests {
		if got := test.role.Meets(test.minimum); got != test.expected {
			t.Fatalf("%q.Meets(%q) = %v, want %v", test.role, test.minimum, got, test.expected)
		}
	}
}
