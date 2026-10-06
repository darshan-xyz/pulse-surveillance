package security

import "strings"

// RBAC Role Constants
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

// RBAC Permission Constants
const (
	PermDeviceView        = "Device:View"
	PermDeviceControl     = "Device:Control"
	PermDeviceManageUsers = "Device:ManageUsers"
	PermStreamAccess      = "Stream:Access"
)

// AllPermissions returns all supported permissions
var AllPermissions = []string{
	PermDeviceView,
	PermDeviceControl,
	PermDeviceManageUsers,
	PermStreamAccess,
}

// RolePermissions defines the role-to-permission mapping matrix
var RolePermissions = map[string][]string{
	RoleAdmin: {
		PermDeviceView,
		PermDeviceControl,
		PermDeviceManageUsers,
		PermStreamAccess,
	},
	RoleOperator: {
		PermDeviceView,
		PermDeviceControl,
		PermStreamAccess,
	},
	RoleViewer: {
		PermDeviceView,
		PermStreamAccess,
	},
}

// ValidateRole returns true if the role is recognized
func ValidateRole(role string) bool {
	norm := strings.ToLower(strings.TrimSpace(role))
	_, exists := RolePermissions[norm]
	return exists
}

// GetRolePermissions returns all permissions assigned to a given role
func GetRolePermissions(role string) []string {
	norm := strings.ToLower(strings.TrimSpace(role))
	perms, exists := RolePermissions[norm]
	if !exists {
		return []string{}
	}
	// Return a copy
	res := make([]string, len(perms))
	copy(res, perms)
	return res
}

// RoleHasPermission checks if a given role contains the requested permission
func RoleHasPermission(role, permission string) bool {
	normRole := strings.ToLower(strings.TrimSpace(role))
	perms, exists := RolePermissions[normRole]
	if !exists {
		return false
	}
	for _, p := range perms {
		if strings.EqualFold(p, permission) {
			return true
		}
	}
	return false
}

// UserHasPermission checks if a set of permissions or role grants the action
func UserHasPermission(userRole string, explicitPerms []string, requiredPerm string) bool {
	if RoleHasPermission(userRole, requiredPerm) {
		return true
	}
	for _, p := range explicitPerms {
		if strings.EqualFold(p, requiredPerm) {
			return true
		}
	}
	return false
}
