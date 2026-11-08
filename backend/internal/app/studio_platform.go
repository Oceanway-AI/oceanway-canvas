package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"github.com/google/uuid"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
)

// NewStudio is the executable's constructor. Missing Platform configuration is
// an initialization error; the upstream constructor remains for domain fixtures.
func NewStudio(repo *repository.Repository, dataDir string, gateway *platformintegration.Gateway) (*Service, error) {
	if gateway == nil || gateway.Client == nil {
		return nil, errors.New("Studio requires Platform configuration")
	}
	s := newService(repo, dataDir)
	s.studioPlatform = gateway
	return s, nil
}

func (s *Service) UsesPlatformIdentity() bool { return s.studioPlatform != nil }

func (s *Service) StudioConsoleURL() string {
	if s.studioPlatform == nil {
		return ""
	}
	return s.studioPlatform.Origin()
}

func (s *Service) ValidateStudioRequest(r *http.Request) error {
	if s.studioPlatform == nil {
		return NewAppError(503, "Platform 尚未配置")
	}
	if err := s.studioPlatform.ValidateInbound(r); err != nil {
		return Forbidden("请求来源未经授权")
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
	subject := s.studioPlatform.Origin() + "\n" + strconv.Itoa(identity.User.ID)
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(subject)).String()
	now := time.Now()
	user, err := s.repo.WithContext(ctx).SyncPlatformUser(model.User{
		ID: id, Username: "platform_" + id, DisplayName: identity.User.DisplayName, Email: identity.User.Email,
		Role: model.UserRoleUser, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}, model.UserIdentity{ID: id, UserID: id, Provider: "oceanway-platform", Subject: fmt.Sprintf("%x", sha256.Sum256([]byte(subject))),
		ProviderUsername: identity.User.Username, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return nil, result.Response, err
	}
	// Role is derived from this response and is intentionally never persisted.
	user.Username = identity.User.Username
	user.PlatformAuthenticated = true
	user.PlatformSessionID = identity.Session.SID
	for _, role := range identity.ProductRoles {
		if role == "studio:admin" {
			user.Role = model.UserRoleAdmin
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
