package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
)

// NewStudio is the executable's constructor. Missing Platform configuration is
// an initialization error.
func NewStudio(repo *repository.Repository, dataDir string, gateway *platformintegration.Gateway) (*Service, error) {
	if gateway == nil || gateway.Client == nil {
		return nil, errors.New("Studio requires Platform configuration")
	}
	if err := repo.RequireStudioSchema(); err != nil {
		return nil, err
	}
	return newService(repo, dataDir, gateway), nil
}

func (s *Service) UsesPlatformIdentity() bool { return s.studioPlatform != nil }

func (s *Service) StudioConsoleURL() string {
	if s.studioPlatform == nil {
		return ""
	}
	return s.studioPlatform.BrowserOrigin()
}

func (s *Service) ValidateStudioRequest(r *http.Request) error {
	if s.studioPlatform == nil {
		return NewAppError(503, "Platform 尚未配置")
	}
	if err := s.studioPlatform.ValidateInbound(r); err != nil {
		return Forbidden("请求来源未经授权")
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && r.Header.Get("X-Studio-Identity-Version") != "platform-id-v1" {
		err := NewAppError(http.StatusConflict, "Studio 身份版本已更新，请刷新页面后重试")
		err.Reason = "studio_identity_version_changed"
		return err
	}
	return nil
}

func (s *Service) StudioLoginURL(ctx context.Context) (string, error) {
	if s.studioPlatform == nil {
		return "", NewAppError(503, "Platform 尚未配置")
	}
	target, err := s.studioPlatform.LoginURL(ctx)
	if err != nil {
		return "", WrapAppError(503, "暂时无法登录，请稍后重试", err)
	}
	return target, nil
}

func (s *Service) ValidateStudioActor(user *model.User, actorID string) error {
	if user == nil || !user.PlatformAuthenticated || actorID == "" || user.ID != actorID {
		err := NewAppError(http.StatusConflict, "账号已切换，请刷新页面后重试")
		err.Reason = "studio_actor_changed"
		return err
	}
	return nil
}

func platformIdentityError(err error) error {
	var upstream *platformclient.Error
	if errors.As(err, &upstream) {
		switch upstream.StatusCode {
		case 401:
			return WrapAppError(401, "请登录 OceanWay 账号", upstream)
		case 403:
			return WrapAppError(403, "当前账号无权访问", upstream)
		case 429:
			return WrapAppError(429, "请求过于频繁，请稍后重试", upstream)
		}
		// The SDK error retains only safe context sentinels, not response bodies
		// or transport details. Preserve cancellation without exposing credentials.
		return WrapAppError(503, "暂时无法验证登录状态，请稍后重试", upstream)
	}
	return NewAppError(503, "暂时无法验证登录状态，请稍后重试")
}

func (s *Service) studioUserID(platformID int) string {
	return strconv.Itoa(platformID)
}

func (s *Service) StudioCurrentUser(ctx context.Context, session platformclient.SessionContext) (*model.User, *platformclient.Response, error) {
	if s.studioPlatform == nil {
		return nil, nil, NewAppError(503, "Platform 尚未配置")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := s.studioPlatform.Client.Identity(ctx, session)
	if err != nil {
		return nil, result.Response, platformIdentityError(err)
	}
	identity := result.Data
	if identity.User.ID <= 0 || identity.User.Status != 1 || identity.Session.SID == "" {
		return nil, result.Response, Unauthorized("登录状态已失效")
	}
	user := &model.User{
		ID: s.studioUserID(identity.User.ID), Username: identity.User.Username,
		DisplayName: identity.User.DisplayName, Email: identity.User.Email,
		Status:                model.UserStatusActive,
		PlatformAuthenticated: true, PlatformAvatarURL: identity.User.AvatarURL,
		PlatformSessionID: identity.Session.SID,
	}
	// PLAT-02-010 delegates product roles to Studio. These permissions cover
	// creative content only and never authorize Platform account or finance APIs.
	for _, role := range identity.ProductRoles {
		if role == "studio:admin" {
			user.StudioPermissions = []model.StudioPermission{
				model.StudioManageCreativeContent,
				model.StudioDeleteBuiltinSkill, model.StudioManageSkillCategories, model.StudioCurateSkills,
			}
			break
		}
	}
	return user, result.Response, nil
}

func (s *Service) StudioLogout(ctx context.Context, session platformclient.SessionContext) (*platformclient.Response, error) {
	if s.studioPlatform == nil {
		return nil, NewAppError(503, "Platform 尚未配置")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := s.studioPlatform.Client.Logout(ctx, session)
	if err != nil {
		return result.Response, platformIdentityError(err)
	}
	return result.Response, nil
}
