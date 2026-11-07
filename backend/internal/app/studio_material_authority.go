package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

func studioMaterialSessionHash(user *model.User, session platformclient.SessionContext) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(user.ID+"\x00"+session.Host+"\x00"+user.PlatformSessionID)))
}

// Bootstrap is an explicit browser POST with its genuine Origin. Read-only file
// GETs later use this revocable, session-bound material grant without cookies.
func (s *Service) StudioAuthorizeMaterials(ctx context.Context, user *model.User, session platformclient.SessionContext) (*platformclient.Response, error) {
	response := &platformclient.Response{Header: make(http.Header)}
	if user == nil || !user.PlatformAuthenticated || user.PlatformSessionID == "" || session.Origin == "" || session.Cookie == "" {
		return response, Forbidden("素材授权需要当前浏览器会话来源")
	}
	key := studioMaterialSessionHash(user, session)
	if _, err := s.repo.WithContext(ctx).StudioMaterialAuthority(user.ID, key); err == nil {
		return response, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return response, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	grant, err := s.studioPlatform.Client.IssueMaterial(ctx, session)
	if grant.Response != nil {
		grant.Response.AppendCookies(response.Header)
	}
	authorization, expires, e := studioMaterialGrant(grant, err)
	if e != nil {
		return response, e
	}
	encrypted, e := s.encryptSettingSecret(authorization)
	if e != nil {
		return response, e
	}
	return response, s.repo.WithContext(ctx).SaveStudioMaterialAuthority(model.StudioMaterialAuthority{SessionHash: key, UserID: user.ID, AuthorizationEncrypted: encrypted, ExpiresAt: expires, UpdatedAt: time.Now()})
}

func studioMaterialGrant(result platformclient.Result[platformclient.MaterialAuthorization], callErr error) (string, time.Time, error) {
	if result.Response == nil || result.Response.StatusCode < 200 || result.Response.StatusCode >= 300 {
		if callErr != nil {
			return "", time.Time{}, studioMaterialError(callErr)
		}
		return "", time.Time{}, NewAppError(502, "素材授权响应无效")
	}
	// rc.5 declares expires_at as string; the published producer uses unix
	// seconds. Decode only this exact successful SDK response, never an error
	// envelope or a credential from another endpoint.
	var grant struct {
		Authorization string          `json:"authorization"`
		ExpiresAt     json.RawMessage `json:"expires_at"`
	}
	if err := json.Unmarshal(result.Response.Body, &grant); err != nil || !strings.HasPrefix(grant.Authorization, "mat1.") || len(grant.Authorization) > 4096 {
		return "", time.Time{}, NewAppError(502, "素材授权响应无效")
	}
	raw := string(grant.ExpiresAt)
	if len(raw) > 0 && raw[0] == '"' {
		if json.Unmarshal(grant.ExpiresAt, &raw) != nil {
			return "", time.Time{}, NewAppError(502, "素材授权期限无效")
		}
	}
	var expires time.Time
	if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil {
		expires = time.Unix(seconds, 0)
	} else {
		expires, _ = time.Parse(time.RFC3339, raw)
	}
	if !expires.After(time.Now().Add(time.Minute)) || expires.After(time.Now().Add(8*24*time.Hour)) {
		return "", time.Time{}, NewAppError(502, "素材授权期限无效")
	}
	return grant.Authorization, expires, nil
}

func (s *Service) studioMaterialReadAuthority(ctx context.Context, user *model.User, session platformclient.SessionContext) (*platformclient.Materials, error) {
	row, err := s.repo.WithContext(ctx).StudioMaterialAuthority(user.ID, studioMaterialSessionHash(user, session))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, &AppError{Status: 409, Code: 409, Reason: "material_authorization_required", Message: "素材会话需要刷新，请重新打开工作区"}
	}
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(row.AuthorizationEncrypted, encryptedSettingPrefix) {
		return nil, NewAppError(503, "素材授权记录不可用")
	}
	auth, err := s.decryptSettingSecret(row.AuthorizationEncrypted)
	if err != nil {
		return nil, NewAppError(503, "素材授权记录不可用")
	}
	return s.studioPlatform.Client.WorkerMaterials(platformclient.MaterialContext{Authorization: auth}), nil
}
