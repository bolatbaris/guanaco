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
	"fmt"
	"net/http"

	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/web/handler"

	"github.com/danielgtaylor/huma/v2"
)

type delegationNameListBody struct {
	Body Paginated[*models.DelegationName]
}

type taskDelegationPathParams struct {
	TaskID int64 `path:"projecttask" doc:"The numeric id of the task whose delegation is being changed."`
}

type taskDelegationCreateInput struct {
	TaskID int64 `path:"projecttask" doc:"The numeric id of the task whose delegation is being changed."`
	Body   models.TaskDelegation
}

func RegisterTaskDelegationRoutes(api huma.API) {
	tags := []string{"delegation"}

	Register(api, huma.Operation{
		OperationID: "delegation-names-list",
		Summary:     "List delegation names",
		Description: "Returns the authenticated user's previously used external delegate names, ordered by usage and filtered by q. Names are private to that user.",
		Method:      http.MethodGet,
		Path:        "/delegation-names",
		Tags:        tags,
	}, delegationNamesList)

	Register(api, huma.Operation{
		OperationID: "tasks-delegation-create",
		Summary:     "Delegate a task",
		Description: "Assigns the task to one external person represented by a full name. The task id comes from the URL, and the authenticated user remains the tracker without creating an external user account.",
		Method:      http.MethodPost,
		Path:        "/tasks/{projecttask}/delegation",
		Tags:        tags,
	}, taskDelegationCreate)

	Register(api, huma.Operation{
		OperationID: "tasks-delegation-delete",
		Summary:     "Take back a delegated task",
		Description: "Clears the external delegation and assigns the task back to the authenticated user as its sole system owner.",
		Method:      http.MethodDelete,
		Path:        "/tasks/{projecttask}/delegation",
		Tags:        tags,
	}, taskDelegationDelete)
}

func init() { AddRouteRegistrar(RegisterTaskDelegationRoutes) }

func delegationNamesList(ctx context.Context, in *struct {
	ListParams
}) (*delegationNameListBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	result, _, total, err := handler.DoReadAll(ctx, &models.DelegationName{}, a, in.Q, in.Page, in.PerPage)
	if err != nil {
		return nil, translateDomainError(err)
	}
	items, ok := result.([]*models.DelegationName)
	if !ok {
		return nil, fmt.Errorf("delegationNames.ReadAll returned unexpected type %T (expected []*models.DelegationName)", result)
	}

	return &delegationNameListBody{Body: NewPaginated(items, total, in.Page, in.PerPage)}, nil
}

func taskDelegationCreate(ctx context.Context, in *taskDelegationCreateInput) (*singleBody[models.Task], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	delegation := in.Body
	delegation.TaskID = in.TaskID // URL wins over body
	if err := handler.DoCreate(ctx, &delegation, a); err != nil {
		return nil, translateDomainError(err)
	}

	task := &models.Task{ID: in.TaskID}
	if _, err := handler.DoReadOne(ctx, task, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.Task]{Body: task}, nil
}

func taskDelegationDelete(ctx context.Context, in *taskDelegationPathParams) (*emptyBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	delegation := &models.TaskDelegation{TaskID: in.TaskID}
	if err := handler.DoDelete(ctx, delegation, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &emptyBody{}, nil
}
