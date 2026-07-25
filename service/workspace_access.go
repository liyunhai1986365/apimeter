package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var ErrWorkspaceAccountUnavailable = errors.New("workspace account is unavailable")

// WorkspaceAccessScope is the single authorization context for every workspace-scoped
// query. The only thing a workspace account changes is that its queries carry an extra
// `workspace_id IN (...)` restriction; ownership of the data never moves.
type WorkspaceAccessScope struct {
	ActorUserId  int  `json:"actor_user_id"`
	OwnerUserId  int  `json:"owner_user_id"`
	IsSubaccount bool `json:"is_subaccount"`
	// WorkspaceIds is only populated for workspace accounts. A main account is
	// unrestricted, so its workspace list is never resolved.
	WorkspaceIds []int `json:"workspace_ids"`
}

func ResolveWorkspaceAccessScope(actorUserId int) (*WorkspaceAccessScope, error) {
	actor, err := model.GetUserCache(actorUserId)
	if err != nil {
		return nil, err
	}
	if actor.Status != common.UserStatusEnabled {
		return nil, ErrWorkspaceAccountUnavailable
	}
	scope := &WorkspaceAccessScope{
		ActorUserId: actorUserId,
		OwnerUserId: actorUserId,
	}
	if actor.ParentUserId == 0 {
		return scope, nil
	}
	owner, err := model.GetUserCache(actor.ParentUserId)
	if err != nil || owner.Status != common.UserStatusEnabled || owner.ParentUserId != 0 {
		return nil, ErrWorkspaceAccountUnavailable
	}
	scope.OwnerUserId = actor.ParentUserId
	scope.IsSubaccount = true
	scope.WorkspaceIds, err = model.ListAccessibleWorkspaceIds(scope.OwnerUserId, actorUserId)
	if err != nil {
		return nil, err
	}
	if scope.WorkspaceIds == nil {
		scope.WorkspaceIds = []int{}
	}
	return scope, nil
}

// WorkspaceFilter is the value every model-layer query takes as allowedWorkspaceIds:
// nil means "no restriction" (main account), a non-nil slice restricts to those ids,
// and an empty non-nil slice restricts to nothing.
func (scope *WorkspaceAccessScope) WorkspaceFilter() []int {
	if scope == nil {
		return []int{}
	}
	if !scope.IsSubaccount {
		return nil
	}
	if scope.WorkspaceIds == nil {
		return []int{}
	}
	return scope.WorkspaceIds
}

func (scope *WorkspaceAccessScope) CanAccessWorkspace(workspaceId int) bool {
	if scope == nil || workspaceId <= 0 {
		return false
	}
	if !scope.IsSubaccount {
		// Ownership is still enforced by every owner-scoped query (user_id = OwnerUserId).
		return true
	}
	for _, id := range scope.WorkspaceIds {
		if id == workspaceId {
			return true
		}
	}
	return false
}

func (scope *WorkspaceAccessScope) RequireWorkspace(workspaceId int) (*model.Workspace, error) {
	if !scope.CanAccessWorkspace(workspaceId) {
		return nil, gormErrRecordNotFound
	}
	return model.GetUserWorkspaceByID(scope.OwnerUserId, workspaceId)
}

var gormErrRecordNotFound = errors.New("workspace not found or unavailable")
