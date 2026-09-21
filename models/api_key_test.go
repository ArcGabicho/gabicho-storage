package models

import "testing"

func TestAPIKey_HasPermission(t *testing.T) {
	cases := []struct {
		name        string
		permissions string
		check       string
		want        bool
	}{
		{"read exacto", "read", PermissionRead, true},
		{"write exacto", "write", PermissionWrite, true},
		{"read no incluye write", "read", PermissionWrite, false},
		{"lista combinada", "read,write", PermissionWrite, true},
		{"admin implica read", "admin", PermissionRead, true},
		{"admin implica write", "admin", PermissionWrite, true},
		{"admin implica admin", "admin", PermissionAdmin, true},
		{"espacios alrededor de comas", "read, write", PermissionWrite, true},
		{"permiso vacío", "", PermissionRead, false},
		{"permiso desconocido no otorga nada", "foo", PermissionRead, false},
	}

	for _, tc := range cases {
		k := &APIKey{Permissions: tc.permissions}
		got := k.HasPermission(tc.check)
		if got != tc.want {
			t.Errorf("%s: HasPermission(%q) with permissions=%q = %v, want %v",
				tc.name, tc.check, tc.permissions, got, tc.want)
		}
	}
}
