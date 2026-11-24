package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"yingce/backend/internal/model"
)

// The shared storage key fits the original 36-character partition columns.
// Platform account IDs occupy a separate, numeric namespace.
func (w StudioWorkspaceEntry) TeamCanvasKey() (string, error) {
	if w.workspace.Kind != "team" || w.workspace.ID == "" || w.workspace.MembershipGeneration <= 0 ||
		w.workspace.Team==nil || w.workspace.Team.WorkspaceID!=w.WorkspaceID || w.workspace.Team.ExpiresAt<=time.Now().Unix() ||
		w.workspace.ID != w.WorkspaceID || w.workspace.MembershipGeneration != w.MembershipGeneration {
		return "", Forbidden("需要有效的团队成员资格")
	}
	raw := sha256.Sum256([]byte(w.workspace.ID))
	return "tw:" + hex.EncodeToString(raw[:16]), nil
}

func (w StudioWorkspaceEntry) teamCanvasProof(user *model.User, proof model.CanvasEditProof) (model.CanvasEditProof, error) {
	if proof.SessionID == "" || len(proof.SessionID) > 160 || user.PlatformSessionID == "" || w.UserID != user.ID {
		return proof, BadAuthRequest("画布编辑会话无效")
	}
	proof.SessionID = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%s", user.ID, user.PlatformSessionID, w.MembershipGeneration, proof.SessionID))))
	return proof, nil
}

func (s *Service) StudioTeamCanvasOperations(user *model.User, w *StudioWorkspaceEntry, id string, req CanvasOperationsRequest) (UserDataSummary, error) {
	key, err := w.TeamCanvasKey()
	if err != nil {
		return UserDataSummary{}, err
	}
	if user.ID != w.UserID {
		return UserDataSummary{}, Forbidden("团队用户不一致")
	}
	for i, proof := range req.EditProofs {
		req.EditProofs[i], err = w.teamCanvasProof(user, proof)
		if err != nil {
			return UserDataSummary{}, err
		}
	}
	return s.canvasDomain().ApplyTeamCanvasOperations(key, w.WorkspaceID, user.ID, w.MembershipGeneration, id, req)
}

func (s *Service) StudioTeamCanvasLease(user *model.User, w *StudioWorkspaceEntry, id string, req CanvasEditLeaseRequest) (map[string]any, error) {
	key, err := w.TeamCanvasKey()
	if err != nil {
		return nil, err
	}
	req.CanvasEditProof, err = w.teamCanvasProof(user, req.CanvasEditProof)
	if err != nil {
		return nil, err
	}
	return s.ChangeUserCanvasEditLease(key, id, req)
}
