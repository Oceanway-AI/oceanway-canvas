package model

import "slices"

type StudioPermission string

const (
	StudioManageCreativeContent StudioPermission = "content:manage"
	StudioDeleteBuiltinSkill    StudioPermission = "skills:builtin:delete"
	StudioManageSkillCategories StudioPermission = "skills:categories:write"
	StudioCurateSkills          StudioPermission = "skills:curation:write"
)

// Permissions are projected from the current Platform response, never stored.
func (u *User) CanStudio(permission StudioPermission) bool {
	return u != nil && u.ID != "" && u.PlatformAuthenticated && u.PlatformSessionID != "" &&
		u.Status == UserStatusActive && slices.Contains(u.StudioPermissions, permission)
}
