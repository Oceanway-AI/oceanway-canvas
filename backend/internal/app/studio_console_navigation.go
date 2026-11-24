package app

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"gorm.io/gorm"
)

var studioConsoleSkillPath = regexp.MustCompile(`^/skills/([A-Za-z0-9_-]{1,80})/edit$`)

var studioConsoleCanvasPath = regexp.MustCompile(`^/canvas/[A-Za-z0-9_-]{1,80}$`)

// Studio currently owns personal canvases. The visible account and payer must
// still match at click time, before preparing any Console return context.
func (s *Service) StudioConsoleNavigation(ctx context.Context, session platformclient.SessionContext, userID, workspaceID, destination, returnPath string) (string, *platformclient.Response, error) {
	if !validStudioConsoleDestination(destination) || userID == "" || workspaceID == "" {
		return "", nil, BadAuthRequest("账户导航参数无效")
	}
	if s.studioPlatform == nil {
		return "", nil, NewAppError(503, "Platform 尚未配置")
	}
	base := s.studioPlatform.PublicBasePath()
	local := strings.TrimPrefix(returnPath, base)
	if base != "" && returnPath != base && !strings.HasPrefix(returnPath, base+"/") {
		return "", nil, BadAuthRequest("返回页面无效")
	}
	canvas := studioConsoleCanvasPath.MatchString(local)
	skill := studioConsoleSkillPath.FindStringSubmatch(local)
	if !canvas && skill == nil && local != "/canvas" && local != "/skills" && local != "/create" && local != "/tasks" && local != "/assets" && local != "/settings" && !(base != "" && local == "" || base == "" && local == "/") {
		return "", nil, BadAuthRequest("返回页面无效")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	account, response, err := s.studioPersonalAccount(ctx, session)
	if err != nil {
		return "", response, err
	}
	if s.studioUserID(account.User.ID) != userID || account.Workspace.ID != workspaceID {
		return "", response, NewAppError(409, "账号或空间已变化，请返回 Studio 刷新后重试")
	}
	if canvas {
		if _, err := s.repo.WithContext(ctx).CanvasProjectMetadataForUser(userID, strings.TrimPrefix(local, "/canvas/")); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", response, NewAppError(404, "画布不存在")
			}
			return "", response, err
		}
	}
	if skill != nil {
		if _, err := s.SkillDetail(userID, skill[1]); err != nil {
			return "", response, err
		}
	}
	section := ""
	if destination == "models" {
		destination, section = "account", "&section=models"
	}
	result, err := s.studioPlatform.Client.ConsoleNavigation(ctx, session, account.Workspace, destination, returnPath)
	if response == nil {
		response = &platformclient.Response{Header: make(http.Header)}
	}
	result.Response.AppendCookies(response.Header)
	if err != nil {
		return "", response, platformIdentityError(err)
	}
	return s.StudioConsoleURL() + result.Data.ConsolePath + section, response, nil
}

func validStudioConsoleDestination(destination string) bool {
	switch destination {
	case "account", "models", "wallet", "usage", "subscriptions", "plans", "orders", "recharge":
		return true
	}
	return false
}
