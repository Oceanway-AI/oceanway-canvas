package app

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

type StudioAccountSummary struct {
	UserID        string                              `json:"userId"`
	DisplayName   string                              `json:"displayName"`
	AvatarURL     string                              `json:"avatarUrl,omitempty"`
	WorkspaceID   string                              `json:"workspaceId"`
	WorkspaceName string                              `json:"workspaceName"`
	Balance       platformclient.AccountBalance       `json:"balance"`
	Applications  []platformclient.AccountApplication `json:"applications"`
	AccountURL    string                              `json:"accountUrl,omitempty"`
	WalletURL     string                              `json:"walletUrl,omitempty"`
	UsageURL      string                              `json:"usageUrl,omitempty"`
}

// The creation surface issues personal execution grants. Use that same payer for
// its account bar until Studio supports team-bound projects and execution.
func (s *Service) StudioAccountSummary(ctx context.Context, session platformclient.SessionContext) (*StudioAccountSummary, *platformclient.Response, error) {
	summary, response, err := s.studioPersonalAccount(ctx, session)
	if err != nil {
		return nil, response, err
	}
	name := summary.User.DisplayName
	if name == "" {
		name = summary.User.Username
	}
	// Registered application links are browser destinations. Platform's Console
	// may be addressed internally by a different origin.
	applications := make([]platformclient.AccountApplication, 0, len(summary.Applications))
	for _, application := range summary.Applications {
		if strings.EqualFold(application.Product, "console") && application.URL == "/console" {
			application.URL = s.StudioConsoleURL() + "/console"
		} else {
			target, parseErr := url.Parse(application.URL)
			if parseErr != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil {
				continue
			}
		}
		applications = append(applications, application)
	}
	return &StudioAccountSummary{
		UserID: s.studioUserID(summary.User.ID), DisplayName: name, AvatarURL: summary.User.AvatarURL,
		WorkspaceID: summary.Workspace.ID, WorkspaceName: summary.Workspace.Name, Balance: summary.Balance,
		Applications: applications, AccountURL: s.studioConsoleLink(summary.AccountPath),
		WalletURL: s.studioConsoleLink(summary.WalletPath), UsageURL: s.studioConsoleLink(summary.UsagePath),
	}, response, nil
}

func (s *Service) studioConsoleLink(path string) string {
	u, err := url.ParseRequestURI(path)
	if err != nil || u.IsAbs() || u.Host != "" || u.Path == "" || path[0] != '/' {
		return ""
	}
	// Only the three published Console destinations are accepted.
	switch u.Path {
	case "/console/account", "/console/wallet", "/console/records":
		return s.StudioConsoleURL() + u.RequestURI()
	}
	return ""
}

func (s *Service) studioPersonalAccount(ctx context.Context, session platformclient.SessionContext) (platformclient.AccountSummary, *platformclient.Response, error) {
	if s.studioPlatform == nil {
		return platformclient.AccountSummary{}, nil, NewAppError(503, "Platform 尚未配置")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := s.studioPlatform.Client.AccountSummary(ctx, session, "")
	if err != nil {
		return platformclient.AccountSummary{}, result.Response, platformIdentityError(err)
	}
	summary := result.Data
	if summary.User.ID <= 0 || summary.User.Status != 1 || summary.Workspace.Kind != "personal" || summary.Workspace.ID == "" || summary.Workspace.ActorUserID != summary.User.ID || summary.Workspace.MembershipGeneration != 0 {
		return platformclient.AccountSummary{}, result.Response, NewAppError(503, "暂时无法确认当前个人空间")
	}
	return summary, result.Response, nil
}
