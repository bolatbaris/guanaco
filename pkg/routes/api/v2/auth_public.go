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

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/routes/api/shared"
	"code.vikunja.io/api/pkg/user"

	"github.com/danielgtaylor/huma/v2"
)

// publicSecurity is the empty security requirement that opts an operation out of
// the globally-applied JWT/API-token auth. The matching Echo path must also be
// listed in unauthenticatedAPIPaths so the token middleware lets it through.
var publicSecurity = []map[string][]string{}

// messageBody carries a human-readable confirmation for endpoints that report
// success without returning a resource.
type messageBody struct {
	Body struct {
		Message string `json:"message" readOnly:"true" doc:"A human-readable confirmation message."`
	}
}

func init() { AddRouteRegistrar(RegisterPublicAuthRoutes) }

// RegisterPublicAuthRoutes wires the unauthenticated email confirmation
// endpoint. Public account creation is intentionally absent; the configured
// single user is provisioned during startup.
func RegisterPublicAuthRoutes(api huma.API) {
	if config.AuthLocalEnabled.GetBool() {
		registerEmailConfirmationRoute(api)
	}
}

func registerEmailConfirmationRoute(api huma.API) {
	authTags := []string{"auth"}

	Register(api, huma.Operation{
		OperationID:   "auth-confirm-email",
		Summary:       "Confirm an email address",
		Description:   "Confirms an email address using the token sent to the account.",
		Method:        http.MethodPost,
		Path:          "/user/confirm",
		DefaultStatus: http.StatusOK,
		Tags:          authTags,
		Security:      publicSecurity,
	}, authConfirmEmail)
}

func authConfirmEmail(_ context.Context, in *struct{ Body user.EmailConfirm }) (*messageBody, error) {
	if err := shared.ConfirmEmail(&in.Body); err != nil {
		return nil, translateDomainError(err)
	}
	out := &messageBody{}
	out.Body.Message = "The email was confirmed successfully."
	return out, nil
}
