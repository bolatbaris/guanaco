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
	"errors"
	"net/http"

	"code.vikunja.io/api/pkg/observability"

	"github.com/danielgtaylor/huma/v2"
	"github.com/getsentry/sentry-go"
)

type observabilityTestBody struct {
	Body struct {
		Captured bool   `json:"captured" readOnly:"true" doc:"Whether the backend sent the test exception to the configured Sentry-compatible service."`
		Message  string `json:"message" readOnly:"true" doc:"A human-readable result of the test request."`
	}
}

// RegisterObservabilityTestRoutes exposes an authenticated diagnostic action
// for verifying the backend's configured Sentry-compatible destination.
func RegisterObservabilityTestRoutes(api huma.API) {
	Register(api, huma.Operation{
		OperationID:   "observability-test-backend",
		Summary:       "Send a backend GlitchTip test event",
		Description:   "Captures a deliberate backend exception with a diagnostic tag. It does not change application data and is available only to authenticated users.",
		Method:        http.MethodPost,
		Path:          "/observability/test/backend",
		DefaultStatus: http.StatusOK,
		Tags:          []string{"observability"},
	}, observabilityBackendTest)
}

func init() { AddRouteRegistrar(RegisterObservabilityTestRoutes) }

func observabilityBackendTest(ctx context.Context, _ *struct{}) (*observabilityTestBody, error) {
	if _, err := authFromCtx(ctx); err != nil {
		return nil, err
	}

	out := &observabilityTestBody{}
	if !observability.Enabled() {
		out.Body.Message = "Backend error tracking is disabled."
		return out, nil
	}

	hub := observability.HubFromContext(ctx)
	eventID := (*sentry.EventID)(nil)
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("glitchtip_test", "true")
		scope.SetTag("component", "backend")
		eventID = hub.CaptureException(errors.New("GlitchTip backend test exception"))
	})

	out.Body.Captured = eventID != nil
	if out.Body.Captured {
		out.Body.Message = "Backend test event sent to GlitchTip."
	} else {
		out.Body.Message = "Backend error tracking did not accept the test event."
	}
	return out, nil
}
