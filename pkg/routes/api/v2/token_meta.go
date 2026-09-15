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

	"code.vikunja.io/api/pkg/models"

	"github.com/danielgtaylor/huma/v2"
)

// apiRoutesBody is the response for the token-routes endpoint: the available
// API routes grouped by permission, for building API-token scopes.
type apiRoutesBody struct {
	Body map[string]models.APITokenRoute
}

func init() { AddRouteRegistrar(RegisterTokenMetaRoutes) }

// RegisterTokenMetaRoutes wires the token introspection helpers and the
// API-token scope discovery endpoint.
func RegisterTokenMetaRoutes(api huma.API) {
	Register(api, huma.Operation{
		OperationID: "token-routes",
		Summary:     "List API token routes",
		Description: "Returns every API route available to scope an API token against, grouped by resource and permission.",
		Method:      http.MethodGet,
		Path:        "/routes",
		Tags:        []string{"api"},
	}, tokenRoutes)
}

func tokenRoutes(_ context.Context, _ *struct{}) (*apiRoutesBody, error) {
	return &apiRoutesBody{Body: models.GetAPITokenRoutes()}, nil
}
