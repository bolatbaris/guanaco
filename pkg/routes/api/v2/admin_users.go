// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package apiv2

import (
	"context"
	"net/http"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/modules/auth/openid"
	"code.vikunja.io/api/pkg/routes/api/shared"
	"code.vikunja.io/api/pkg/user"

	"github.com/danielgtaylor/huma/v2"
	"xorm.io/xorm"
)

type adminOverviewBody struct {
	Body *models.Overview
}

type adminUserBody struct {
	Body *shared.AdminUser
}

type adminUserListBody struct {
	Body Paginated[*shared.AdminUser]
}

// adminIsAdminPatchBody uses a pointer so an omitted is_admin leaves the flag unchanged
// instead of silently demoting.
type adminIsAdminPatchBody struct {
	IsAdmin *bool `json:"is_admin" doc:"New admin flag. Omitting it leaves the current value unchanged."`
}

// adminStatusPatchBody uses a pointer so an omitted status leaves the account unchanged
// instead of silently reactivating.
type adminStatusPatchBody struct {
	Status *user.Status `json:"status" doc:"New account status (0=active, 1=email-confirmation required, 2=disabled, 3=locked). Omitting it leaves the current value unchanged."`
}

// adminDoerFromCtx resolves the acting admin for audit attribution. The admin
// gate guarantees the principal is a user, so anything else is a hard error.
func adminDoerFromCtx(ctx context.Context) (*user.User, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	return user.GetFromAuth(a)
}

// Permissions are enforced by the gateV2AdminRoutes path middleware, not per-handler.
func RegisterAdminUserRoutes(api huma.API) {
	tags := []string{"admin"}

	Register(api, huma.Operation{
		OperationID: "admin-overview",
		Summary:     "Admin overview",
		Description: "Returns per-instance counts (users, projects, tasks, shares) plus the current license snapshot. Restricted to instance admins on a licensed instance; unlicensed or non-admin callers get a 404, making the endpoint indistinguishable from one that is not registered.",
		Method:      http.MethodGet,
		Path:        "/admin/overview",
		Tags:        tags,
	}, adminOverview)

	Register(api, huma.Operation{
		OperationID: "admin-users-list",
		Summary:     "List users (admin)",
		Description: "Returns every user on the instance, paginated, with q matched against username and email. Exposes fields hidden from the normal user API (is_admin, status, auth provider). Each call is recorded in the audit log, since the response contains every user's email address. Restricted to instance admins on a licensed instance; unlicensed or non-admin callers get a 404, making the endpoint indistinguishable from one that is not registered.",
		Method:      http.MethodGet,
		Path:        "/admin/users",
		Tags:        tags,
	}, adminUsersList)

	Register(api, huma.Operation{
		OperationID: "admin-users-patch-admin",
		Summary:     "Promote or demote a user (admin)",
		Description: "Sets a user's instance-admin flag. The body field is a pointer: omitting is_admin leaves the flag unchanged. Demoting the last remaining admin is refused with 400.",
		Method:      http.MethodPatch,
		Path:        "/admin/users/{id}/admin",
		Tags:        tags,
	}, adminUsersPatchAdmin)

	Register(api, huma.Operation{
		OperationID: "admin-users-patch-status",
		Summary:     "Set a user's status (admin)",
		Description: "Changes a user's account status without requiring them to log in. The body field is a pointer: omitting status leaves it unchanged. Moving the last remaining admin out of Active is refused with 400.",
		Method:      http.MethodPatch,
		Path:        "/admin/users/{id}/status",
		Tags:        tags,
	}, adminUsersPatchStatus)

	Register(api, huma.Operation{
		OperationID: "admin-users-delete",
		Summary:     "Delete a user (admin)",
		Description: "Deletes a user. With mode=now the user is removed immediately. With mode=scheduled (the default) the user is scheduled for deletion through the email-confirmation self-deletion flow. Deleting the last remaining admin is refused with 400.",
		Method:      http.MethodDelete,
		Path:        "/admin/users/{id}",
		Tags:        tags,
	}, adminUsersDelete)
}

func init() { AddRouteRegistrar(RegisterAdminUserRoutes) }

func adminOverview(_ context.Context, _ *struct{}) (*adminOverviewBody, error) {
	s := db.NewSession()
	defer s.Close()

	overview, err := models.BuildOverview(s)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &adminOverviewBody{Body: overview}, nil
}

func adminUsersList(ctx context.Context, in *ListParams) (*adminUserListBody, error) {
	doer, err := adminDoerFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	s := db.NewSession()
	defer s.Close()

	users, total, err := models.ListUsersAsAdmin(s, doer, in.Q, in.Page, in.PerPage)
	if err != nil {
		_ = s.Rollback()
		events.CleanupPending(s)
		return nil, translateDomainError(err)
	}
	if err := s.Commit(); err != nil {
		events.CleanupPending(s)
		return nil, translateDomainError(err)
	}
	events.DispatchPending(ctx, s)

	providers, err := openid.GetAllProviders() //nolint:contextcheck // GetAllProviders reads a cached map and takes no context.
	if err != nil {
		return nil, translateDomainError(err)
	}
	out := make([]*shared.AdminUser, 0, len(users))
	for _, u := range users {
		out = append(out, shared.NewAdminUser(u, providers)) //nolint:contextcheck // OIDC provider init deliberately uses a background context — provider lifetime exceeds the request
	}
	return &adminUserListBody{Body: NewPaginated(out, total, in.Page, in.PerPage)}, nil
}

func adminUsersPatchAdmin(ctx context.Context, in *struct {
	ID   int64 `path:"id" doc:"The numeric ID of the user."`
	Body adminIsAdminPatchBody
}) (*adminUserBody, error) {
	if in.Body.IsAdmin == nil {
		return nil, translateDomainError(models.ErrInvalidData{Message: "is_admin is required"})
	}
	return adminCommitUser(ctx, func(s *xorm.Session, doer *user.User) (*user.User, error) { //nolint:contextcheck // see adminCommitUser.
		return models.SetUserAdminFlag(s, doer, in.ID, *in.Body.IsAdmin)
	})
}

func adminUsersPatchStatus(ctx context.Context, in *struct {
	ID   int64 `path:"id" doc:"The numeric ID of the user."`
	Body adminStatusPatchBody
}) (*adminUserBody, error) {
	if in.Body.Status == nil {
		return nil, translateDomainError(models.ErrInvalidData{Message: "status is required"})
	}
	newStatus := *in.Body.Status
	if newStatus < user.StatusActive || newStatus > user.StatusAccountLocked {
		return nil, translateDomainError(models.ErrInvalidData{Message: "invalid status"})
	}
	return adminCommitUser(ctx, func(s *xorm.Session, doer *user.User) (*user.User, error) { //nolint:contextcheck // see adminCommitUser.
		return models.SetUserStatusAsAdmin(s, doer, in.ID, newStatus)
	})
}

func adminUsersDelete(ctx context.Context, in *struct {
	ID   int64  `path:"id" doc:"The numeric ID of the user."`
	Mode string `query:"mode" doc:"'now' deletes immediately; 'scheduled' (the default) triggers the email-confirmation self-deletion flow."`
}) (*emptyBody, error) {
	mode := in.Mode
	if mode == "" {
		mode = "scheduled"
	}
	if mode != "now" && mode != "scheduled" {
		return nil, translateDomainError(models.ErrInvalidData{Message: "invalid mode, expected 'now' or 'scheduled'"})
	}

	doer, err := adminDoerFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	s := db.NewSession()
	defer s.Close()
	if err := models.DeleteUserAsAdmin(s, doer, in.ID, mode); err != nil {
		_ = s.Rollback()
		events.CleanupPending(s)
		return nil, translateDomainError(err)
	}
	if err := s.Commit(); err != nil {
		events.CleanupPending(s)
		return nil, translateDomainError(err)
	}
	events.DispatchPending(ctx, s)
	return &emptyBody{}, nil
}

// adminCommitUser runs a user-returning admin action in its own transaction and
// renders the admin user view. The action does the load/guard/mutate against the
// session (shared with v1 via the models layer); this owns the commit and response.
func adminCommitUser(ctx context.Context, action func(s *xorm.Session, doer *user.User) (*user.User, error)) (*adminUserBody, error) {
	doer, err := adminDoerFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	s := db.NewSession()
	defer s.Close()

	target, err := action(s, doer)
	if err != nil {
		_ = s.Rollback()
		events.CleanupPending(s)
		return nil, translateDomainError(err)
	}
	if err := s.Commit(); err != nil {
		events.CleanupPending(s)
		return nil, translateDomainError(err)
	}
	events.DispatchPending(ctx, s)

	providers, err := openid.GetAllProviders() //nolint:contextcheck // GetAllProviders reads a cached map and takes no context.
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &adminUserBody{Body: shared.NewAdminUser(target, providers)}, nil //nolint:contextcheck // OIDC provider init deliberately uses a background context — provider lifetime exceeds the request
}
