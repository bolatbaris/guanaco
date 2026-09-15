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

type apiTokenListBody struct {
	Body Paginated[*models.APIToken]
}

func RegisterAPITokenRoutes(api huma.API) {
	tags := []string{"tokens"}

	Register(api, huma.Operation{
		OperationID: "tokens-list",
		Summary:     "List api tokens",
		Description: "Returns the api tokens owned by the authenticated user.",
		Method:      http.MethodGet,
		Path:        "/tokens",
		Tags:        tags,
	}, apiTokensList)

	Register(api, huma.Operation{
		OperationID: "tokens-create",
		Summary:     "Create an api token",
		Description: "Creates an api token for the authenticated user. The cleartext token is returned once in this response and is never readable again.",
		Method:      http.MethodPost,
		Path:        "/tokens",
		Tags:        tags,
	}, apiTokensCreate)

	Register(api, huma.Operation{
		OperationID: "tokens-delete",
		Summary:     "Delete an api token",
		Description: "Deletes one of the authenticated user's api tokens.",
		Method:      http.MethodDelete,
		Path:        "/tokens/{id}",
		Tags:        tags,
	}, apiTokensDelete)
}

func init() { AddRouteRegistrar(RegisterAPITokenRoutes) }

func apiTokensList(ctx context.Context, in *ListParams) (*apiTokenListBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	result, _, total, err := handler.DoReadAll(ctx, &models.APIToken{}, a, in.Q, in.Page, in.PerPage)
	if err != nil {
		return nil, translateDomainError(err)
	}
	items, ok := result.([]*models.APIToken)
	if !ok {
		return nil, fmt.Errorf("tokens.ReadAll returned unexpected type %T (expected []*models.APIToken)", result)
	}
	return &apiTokenListBody{Body: NewPaginated(items, total, in.Page, in.PerPage)}, nil
}

func apiTokensCreate(ctx context.Context, in *struct {
	Body models.APIToken
}) (*singleBody[models.APIToken], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if err := handler.DoCreate(ctx, &in.Body, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.APIToken]{Body: &in.Body}, nil
}

func apiTokensDelete(ctx context.Context, in *struct {
	ID int64 `path:"id"`
}) (*emptyBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if err := handler.DoDelete(ctx, &models.APIToken{ID: in.ID}, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &emptyBody{}, nil
}
