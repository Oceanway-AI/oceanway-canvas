package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

type StudioWorkspaceEntry struct {
	UserID               string `json:"userId"`
	WorkspaceID          string `json:"workspaceId"`
	WorkspaceKind        string `json:"workspaceKind"`
	WorkspaceName        string `json:"workspaceName"`
	MembershipGeneration int64  `json:"membershipGeneration"`
	Supported            bool   `json:"supported"`
	PickerSession        string `json:"pickerSession,omitempty"`
	workspace            platformclient.WorkspaceContext
}

// Selection is only a new-entry preference. Persisted objects keep their own
// authority; browser workspace IDs and generations are untrusted expectations.
func (s *Service) StudioWorkspaceContext(ctx context.Context, session platformclient.SessionContext, user *model.User, workspaceID, canvasID string, expectedGeneration *int64) (*StudioWorkspaceEntry, *platformclient.Response, error) {
	response := &platformclient.Response{Header: make(http.Header)}
	if s.studioPlatform == nil || user == nil || !user.PlatformAuthenticated {
		return nil, response, Unauthorized("请登录 OceanWay 账号")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client := s.studioPlatform.Client
	personal := false
	if canvasID != "" {
		_, err := s.repo.WithContext(ctx).CanvasProjectMetadataForUser(user.ID, canvasID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, response, NotFound("画布不存在或无权访问")
		}
		if err != nil {
			return nil, response, err
		}
		// The current schema only contains personal canvases. ST2 adds explicit
		// team ownership; never infer it from a URL or the creator's current team.
		personal = true
		expectedGeneration = nil
	}
	if !personal && workspaceID == "" {
		selection, err := client.WorkspaceSelection(ctx, session)
		selection.Response.AppendCookies(response.Header)
		if err != nil {
			return nil, response, platformIdentityError(err)
		}
		value := selection.Data
		if value.ActorUserID <= 0 || s.studioUserID(value.ActorUserID) != user.ID {
			return nil, response, NewAppError(503, "暂时无法确认当前空间")
		}
		switch value.State {
		case "unset":
			personal = true
		case "available":
			if value.Workspace == nil || value.Workspace.ID != value.WorkspaceID || value.Workspace.ActorUserID != value.ActorUserID || value.Workspace.MembershipGeneration != value.MembershipGeneration {
				return nil, response, NewAppError(503, "暂时无法确认当前空间")
			}
			workspaceID = value.WorkspaceID
			expectedGeneration = &value.MembershipGeneration
		case "unavailable":
			return nil, response, NewAppError(409, "原空间已不可用，请在 Console 重新选择空间")
		default:
			return nil, response, NewAppError(503, "暂时无法确认当前空间")
		}
	}
	if personal {
		page, err := client.Workspaces(ctx, session, platformclient.WorkspaceQuery{Limit: 1})
		page.Response.AppendCookies(response.Header)
		if err != nil {
			return nil, response, platformIdentityError(err)
		}
		workspaceID = page.Data.PersonalWorkspaceID
	}
	result, err := client.WorkspaceContext(ctx, session, workspaceID)
	result.Response.AppendCookies(response.Header)
	if err != nil {
		return nil, response, platformIdentityError(err)
	}
	w := result.Data
	if w.ID != workspaceID || w.ID == "" || w.ActorUserID <= 0 || s.studioUserID(w.ActorUserID) != user.ID || (w.Kind != "personal" && w.Kind != "team") || (w.Kind == "personal" && w.MembershipGeneration != 0) || (w.Kind == "team" && w.MembershipGeneration <= 0) || (personal && w.Kind != "personal") {
		return nil, response, NewAppError(503, "暂时无法确认当前空间")
	}
	if w.Status != "active" || !slices.Contains(w.Actions, "workspace:read") {
		return nil, response, Forbidden("当前空间已不可用")
	}
	if expectedGeneration != nil && *expectedGeneration != w.MembershipGeneration {
		return nil, response, NewAppError(409, "团队成员状态已变化，请重新进入空间")
	}
	entry := &StudioWorkspaceEntry{UserID: user.ID, WorkspaceID: w.ID, WorkspaceKind: w.Kind, WorkspaceName: w.Name, MembershipGeneration: w.MembershipGeneration, Supported: w.Kind == "personal", workspace: w}
	if user.PlatformSessionID != "" {
		sum := sha256.Sum256([]byte("studio:picker:" + user.PlatformSessionID))
		entry.PickerSession = hex.EncodeToString(sum[:])
	}
	return entry, response, nil
}
