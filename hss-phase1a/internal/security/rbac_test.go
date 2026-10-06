package security

import "testing"

func TestRBACRolesAndPermissions(t *testing.T) {
	// 1. Admin Role Permissions
	if !RoleHasPermission(RoleAdmin, PermDeviceView) {
		t.Fatal("Admin must have Device:View")
	}
	if !RoleHasPermission(RoleAdmin, PermDeviceControl) {
		t.Fatal("Admin must have Device:Control")
	}
	if !RoleHasPermission(RoleAdmin, PermDeviceManageUsers) {
		t.Fatal("Admin must have Device:ManageUsers")
	}
	if !RoleHasPermission(RoleAdmin, PermStreamAccess) {
		t.Fatal("Admin must have Stream:Access")
	}

	// 2. Operator Role Permissions
	if !RoleHasPermission(RoleOperator, PermDeviceView) {
		t.Fatal("Operator must have Device:View")
	}
	if !RoleHasPermission(RoleOperator, PermDeviceControl) {
		t.Fatal("Operator must have Device:Control")
	}
	if !RoleHasPermission(RoleOperator, PermStreamAccess) {
		t.Fatal("Operator must have Stream:Access")
	}
	if RoleHasPermission(RoleOperator, PermDeviceManageUsers) {
		t.Fatal("Operator must NOT have Device:ManageUsers")
	}

	// 3. Viewer Role Permissions
	if !RoleHasPermission(RoleViewer, PermDeviceView) {
		t.Fatal("Viewer must have Device:View")
	}
	if !RoleHasPermission(RoleViewer, PermStreamAccess) {
		t.Fatal("Viewer must have Stream:Access")
	}
	if RoleHasPermission(RoleViewer, PermDeviceControl) {
		t.Fatal("Viewer must NOT have Device:Control")
	}
	if RoleHasPermission(RoleViewer, PermDeviceManageUsers) {
		t.Fatal("Viewer must NOT have Device:ManageUsers")
	}
}

func TestValidateRole(t *testing.T) {
	validRoles := []string{"admin", "ADMIN", "operator", "Viewer", "viewer"}
	for _, r := range validRoles {
		if !ValidateRole(r) {
			t.Fatalf("Role %s should be valid", r)
		}
	}

	invalidRoles := []string{"", "guest", "superadmin", "hacker"}
	for _, r := range invalidRoles {
		if ValidateRole(r) {
			t.Fatalf("Role %s should be invalid", r)
		}
	}
}

func TestUserHasPermission(t *testing.T) {
	// Viewer with explicit Device:Control grant
	if !UserHasPermission(RoleViewer, []string{PermDeviceControl}, PermDeviceControl) {
		t.Fatal("User with explicit permission grant should be authorized")
	}

	// Viewer without explicit grant attempting Device:Control
	if UserHasPermission(RoleViewer, []string{}, PermDeviceControl) {
		t.Fatal("Viewer without explicit grant should be denied Device:Control")
	}
}
