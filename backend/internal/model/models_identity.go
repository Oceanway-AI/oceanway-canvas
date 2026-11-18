package model

import "time"

type User struct {
	StudioPermissions     []StudioPermission `json:"studioPermissions,omitempty" gorm:"-"`
	PlatformAvatarURL     string             `json:"avatarUrl,omitempty" gorm:"-"`
	PlatformAuthenticated bool               `json:"-" gorm:"-"`
	PlatformSessionID     string             `json:"-" gorm:"-"`
	ID                    string             `json:"id" gorm:"primaryKey;size:36"`
	Username              string             `json:"username" gorm:"uniqueIndex;size:80"`
	Email                 string             `json:"email,omitempty" gorm:"size:160"`
	DisplayName           string             `json:"displayName" gorm:"size:80"`
	Status                UserStatus         `json:"status" gorm:"index;size:24"`
	// UserGroup 是用户分组编码；空串归入 default 分组。
	UserGroup string    `json:"userGroup" gorm:"size:40;not null;default:'';index"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
