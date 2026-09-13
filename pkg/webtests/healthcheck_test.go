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

package webtests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/files"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/routes"
	"code.vikunja.io/api/pkg/user"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCriticalSmokeEnv(t *testing.T) *echo.Echo {
	t.Helper()

	config.InitDefaultConfig()
	config.ServicePublicURL.Set("https://localhost")
	log.InitLogger()

	files.InitTests()
	user.InitTests()
	models.SetupTests()
	require.NoError(t, db.LoadFixtures())

	e := routes.NewEcho()
	routes.RegisterRoutes(e)
	return e
}

func TestCriticalSmoke(t *testing.T) {
	e := setupCriticalSmokeEnv(t)

	healthResponse := httptest.NewRecorder()
	e.ServeHTTP(healthResponse, httptest.NewRequest(http.MethodGet, "/health", nil))
	require.Equal(t, http.StatusOK, healthResponse.Code)
	assert.Contains(t, healthResponse.Body.String(), "OK")

	loginRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/login",
		strings.NewReader(`{"username":"user1","password":"12345678"}`),
	)
	loginRequest.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	loginResponse := httptest.NewRecorder()
	e.ServeHTTP(loginResponse, loginRequest)
	require.Equal(t, http.StatusOK, loginResponse.Code, loginResponse.Body.String())
	assert.Contains(t, loginResponse.Body.String(), "token")
}
