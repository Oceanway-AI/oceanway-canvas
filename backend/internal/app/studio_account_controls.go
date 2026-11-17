package app

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
)

type StudioWorkspaceChoice struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Kind                 string `json:"kind"`
	MembershipGeneration int64  `json:"membershipGeneration"`
	Supported            bool   `json:"supported"`
}

type StudioWorkspaceChoices struct {
	UserID string                  `json:"userId"`
	Items  []StudioWorkspaceChoice `json:"items"`
}

// This list does not change the Platform preference or an open canvas. Team
// choices remain unavailable until the host's team persistence is implemented.
func (s *Service) StudioWorkspaceChoices(ctx context.Context, session platformclient.SessionContext, user *model.User) (*StudioWorkspaceChoices, *platformclient.Response, error) {
	response := &platformclient.Response{Header: make(http.Header)}
	if s.studioPlatform == nil || user == nil || !user.PlatformAuthenticated {
		return nil, response, Unauthorized("请登录 OceanWay 账号")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out := &StudioWorkspaceChoices{UserID: user.ID, Items: []StudioWorkspaceChoice{}}
	cursor := ""
	seenCursors, seenIDs := map[string]bool{}, map[string]bool{}
	for page := 0; page < 100; page++ {
		result, err := s.studioPlatform.Client.Workspaces(ctx, session, platformclient.WorkspaceQuery{Cursor: cursor, Limit: 100})
		result.Response.AppendCookies(response.Header)
		if err != nil {
			return nil, response, platformIdentityError(err)
		}
		for _, w := range result.Data.Items {
			if w.ID == "" || seenIDs[w.ID] || w.ActorUserID <= 0 || s.studioUserID(w.ActorUserID) != user.ID || (w.Kind != "personal" && w.Kind != "team") || (w.Kind == "personal" && w.MembershipGeneration != 0) || (w.Kind == "team" && w.MembershipGeneration <= 0) {
				return nil, response, NewAppError(503, "暂时无法读取空间列表")
			}
			seenIDs[w.ID] = true
			if w.Status == "active" && slices.Contains(w.Actions, "workspace:read") {
				out.Items = append(out.Items, StudioWorkspaceChoice{ID: w.ID, Name: w.Name, Kind: w.Kind, MembershipGeneration: w.MembershipGeneration, Supported: w.Kind == "personal"})
			}
		}
		cursor = result.Data.NextCursor
		if cursor == "" {
			return out, response, nil
		}
		if seenCursors[cursor] {
			break
		}
		seenCursors[cursor] = true
	}
	return nil, response, NewAppError(503, "暂时无法读取完整空间列表")
}

// Refresh the authorized catalog at click time. Never redirect to a client URL
// or treat the global default as the open page's workspace.
func (s *Service) StudioProductNavigation(ctx context.Context, session platformclient.SessionContext, user *model.User, actorID, workspaceID, product string, generation int64) (string, *platformclient.Response, error) {
	if user == nil || user.ID != actorID {
		return "", nil, NewAppError(409, "账号已变化，请刷新后重试")
	}
	summary, response, err := s.StudioAccountSummary(ctx, session, user, workspaceID, "", &generation)
	if err != nil {
		return "", response, err
	}
	for _, app := range summary.Applications {
		if app.Product != product {
			continue
		}
		target, err := url.Parse(app.URL)
		if err != nil || target.Scheme != "https" || target.Host == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
			break
		}
		query := url.Values{"workspace_id": {summary.WorkspaceID}, "membership_generation": {strconv.FormatInt(summary.MembershipGeneration, 10)}}
		target.RawQuery = query.Encode()
		return target.String(), response, nil
	}
	return "", response, Forbidden("此产品暂不可用")
}
