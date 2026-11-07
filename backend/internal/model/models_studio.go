package model

import "time"

// SessionHash is a one-way digest of the current session context. Neither the
// cookie nor the delegated materials credential is exposed through public JSON.
type StudioMaterialAuthority struct {
	SessionHash            string    `gorm:"primaryKey;size:64"`
	UserID                 string    `gorm:"index;size:36"`
	AuthorizationEncrypted string    `gorm:"type:text;not null"`
	ExpiresAt              time.Time `gorm:"index;not null"`
	UpdatedAt              time.Time
}

// StudioExecution is server-only. Envelope includes the frozen native bytes;
// authorization and native responses must never enter Task.InputJSON or logs.
type StudioExecution struct {
	TaskID                 string `gorm:"primaryKey;size:36"`
	UserID                 string `gorm:"size:36;uniqueIndex:idx_studio_execution_command,priority:1"`
	CommandID              string `gorm:"size:128;uniqueIndex:idx_studio_execution_command,priority:2"`
	IntentHash             string `gorm:"size:64;not null"`
	EnvelopeEncrypted      string `gorm:"type:text;not null"`
	AuthorizationEncrypted string `gorm:"type:text"`
	ResponseEncrypted      string `gorm:"type:text"`
	State                  string `gorm:"size:32;index"`
	Released               bool   `gorm:"not null;default:false;index"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

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
