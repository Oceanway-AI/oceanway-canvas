package model

import "time"

// StudioMaterialUpload is an upload intent, never an object store ticket or file.
// Its ID and the Platform command are stable across ambiguous HTTP responses.
type StudioMaterialUpload struct {
	ID         string `gorm:"primaryKey;size:36"`
	UserID     string `gorm:"index;size:36"`
	CommandID  string `gorm:"uniqueIndex;size:80"`
	IntentJSON string `gorm:"type:text;not null"`
	UploadID   string `gorm:"size:128"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// A Resource pins one immutable version. CurrentVersionID and signed URLs are
// deliberately absent: a newer asset version cannot replace an existing canvas.
type StudioMaterialReference struct {
	ResourceID string `gorm:"primaryKey;size:36"`
	UserID     string `gorm:"index;size:36;uniqueIndex:idx_studio_material_version,priority:1"`
	AssetID    string `gorm:"size:128;uniqueIndex:idx_studio_material_version,priority:2"`
	VersionID  string `gorm:"size:128;uniqueIndex:idx_studio_material_version,priority:3"`
	FileID     string `gorm:"size:128"`
	OwnerID    string `gorm:"size:80"`
	SHA256     string `gorm:"size:64"`
	ByteLength string `gorm:"size:32"`
	BindingID  string `gorm:"size:128"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
