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

// @title Vikunja API
// @description This is the documentation for the single-user productivity API. <!-- ReDoc-Inject: <security-definitions> -->

// @description # Pagination
// @description Paginated v2 responses use the standard `{items, total, page, per_page, total_pages}` envelope.
// @description # Permissions
// @description Resource responses expose `max_permission` when permission data is requested. The value is `0` for `Read Only`, `1` for `Read & Write` and `2` for `Admin`.
// @description This can be used to show or hide UI elements based on the permissions the user has.
// @description # Errors
// @description All errors have an error code and a human-readable error message in addition to the http status code. You should always check for the status code in the response, not only the http status code.
// @description Due to limitations in the swagger library we're using for this document, only one error per http status code is documented here. Make sure to check the [error docs](https://vikunja.io/docs/errors/) in Vikunja's documentation for a full list of available error codes.
// @description # Authorization
// @description **JWT-Auth:** Main authorization method, used for most of the requests. Needs `Authorization: Bearer <jwt-token>`-header to authenticate successfully.
// @description
// @description **API Token:** You can create scoped API tokens for your user and use the token to make authenticated requests in the context of that user. The token must be provided via an `Authorization: Bearer <token>` header, similar to jwt auth. See the documentation for the `api` group to manage token creation and revocation.
// @description
// @description **BasicAuth:** Only used when requesting tasks via CalDAV.
// @description <!-- ReDoc-Inject: <security-definitions> -->
// @BasePath /api/v2

// @license.url https://code.vikunja.io/api/src/branch/main/LICENSE
// @license.name AGPL-3.0-or-later

// @contact.url https://vikunja.io/contact/
// @contact.name General Vikunja contact
// @contact.email hello@vikunja.io

// @securityDefinitions.basic BasicAuth

// @securityDefinitions.apikey JWTKeyAuth
// @in header
// @name Authorization

package routes

import (
	"context"
	"log/slog"
	"strings"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/license"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/modules/auth"
	"code.vikunja.io/api/pkg/observability"
	apiv2 "code.vikunja.io/api/pkg/routes/api/v2"
	"code.vikunja.io/api/pkg/routes/caldav"
	"code.vikunja.io/api/pkg/routes/feeds"
	vmiddleware "code.vikunja.io/api/pkg/routes/middleware"
	ws "code.vikunja.io/api/pkg/websocket"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

// matchCORSOrigin checks if an origin matches any of the allowed origin patterns.
// It supports wildcards in the port position (e.g., "http://127.0.0.1:*").
func matchCORSOrigin(origin string, allowedOrigins []string) (string, bool, error) {
	for _, pattern := range allowedOrigins {
		// Exact match
		if origin == pattern {
			return origin, true, nil
		}
		// Allow all
		if pattern == "*" {
			return origin, true, nil
		}
		// Handle wildcard port patterns like "http://127.0.0.1:*" or "http://localhost:*"
		if strings.HasSuffix(pattern, ":*") {
			prefix := strings.TrimSuffix(pattern, ":*")
			// Check if the origin starts with the prefix and has a port after
			if strings.HasPrefix(origin, prefix+":") {
				return origin, true, nil
			}
			// Also match if origin has no port but pattern allows any port
			if origin == prefix {
				return origin, true, nil
			}
		}
	}
	return "", false, nil
}

// NewEcho registers a new Echo instance
func NewEcho() *echo.Echo {
	// Configure Echo with a router that unescapes path parameters.
	// This is needed because Echo v5 does not unescape path params by default.
	// Without this, path parameters like usernames with spaces or apostrophes
	// would remain URL-encoded (e.g., "John%20D%27Urso" instead of "John D'Urso").
	// See https://kolaente.dev/vikunja/vikunja/issues/1224
	e := echo.NewWithConfig(echo.Config{
		Router: echo.NewRouter(echo.RouterConfig{
			UnescapePathParamValues: true,
		}),
		// Since echo v5.3.0 groups implicitly register 404 routes when middleware
		// is added. Our route setup creates multiple groups with the same prefix
		// (e.g. rate-limit subgroups of /api/v2), which would panic as duplicates.
		NoGroupAutoRegister404Routes: true,
	})

	e.IPExtractor = newIPExtractor(config.ServiceIPExtractionMethod.GetString(), config.ServiceTrustedProxies.GetString())

	e.Logger = log.NewEchoLogger(config.LogEnabled.GetBool(), config.LogHTTP.GetString(), config.LogHTTPLevel.GetString(), config.LogFormat.GetString())

	// First middleware in the chain so every request has an ID — reuses the
	// X-Request-Id header from a proxy or generates one — and everything
	// downstream (logging, audit) sees the same value.
	e.Use(middleware.RequestID())

	// Logger
	if config.LogEnabled.GetBool() && config.LogHTTP.GetString() != "off" {
		httpLogger := log.NewHTTPLogger(config.LogEnabled.GetBool(), config.LogHTTP.GetString(), config.LogHTTPLevel.GetString(), config.LogFormat.GetString())
		e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
			LogStatus:    true,
			LogURI:       true,
			LogMethod:    true,
			LogLatency:   true,
			LogRemoteIP:  true,
			LogUserAgent: true,
			LogRequestID: true,
			HandleError:  true,
			LogValuesFunc: func(_ *echo.Context, v middleware.RequestLoggerValues) error {
				attrs := []slog.Attr{
					slog.String("remote_ip", v.RemoteIP),
					slog.String("method", v.Method),
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
					slog.Duration("latency", v.Latency),
					slog.String("user_agent", v.UserAgent),
				}
				if v.RequestID != "" {
					attrs = append(attrs, slog.String("request_id", v.RequestID))
				}
				if v.Error != nil {
					attrs = append(attrs, slog.String("err", v.Error.Error()))
				}
				httpLogger.LogAttrs(context.Background(), httpLogLevel(v.Status), "", attrs...)
				return nil
			},
		}))
	}

	// panic recover
	e.Use(middleware.Recover())

	// Normalize PHP-style `foo[]=...` query params to `foo=...` before any
	// handler binds them. Runs globally so every API request benefits.
	e.Use(vmiddleware.NormalizeArrayParams())

	if config.AuditEnabled.GetBool() {
		e.Use(vmiddleware.RequestMeta())
	}

	setupSentry(e)

	// Validation
	e.Validator = &CustomValidator{}

	// Set body limit to allow file uploads up to the configured size
	// Add some overhead for multipart form data (headers, boundaries, etc.)
	maxFileSize := config.GetMaxFileSizeInMBytes()
	// #nosec G115 - maxFileSize is a configuration value that won't exceed int64 max in practice
	e.Use(middleware.BodyLimit((int64(maxFileSize) + 2) * 1024 * 1024))

	// Set up centralized error handler
	e.HTTPErrorHandler = CreateHTTPErrorHandler(e, observability.Enabled())

	return e
}

func setupSentry(e *echo.Echo) {
	if err := observability.Init(); err != nil {
		log.Criticalf("Sentry init failed: %s", err)
		return
	}
	if !observability.Enabled() {
		return
	}

	e.Use(SentryMiddleware(SentryOptions{
		Repanic: true,
	}))
}

// RegisterRoutes registers all routes for the application
func RegisterRoutes(e *echo.Echo) {

	// One instance keeps every BasicAuth route on the same failure budget.
	noAuthRateLimit := unauthRateLimit()
	refreshRateLimit := tokenRefreshRateLimit()
	basicAuthRateLimit := basicAuthRateLimit()

	if config.ServiceEnableCaldav.GetBool() {
		// Caldav routes
		wkg := e.Group("/.well-known")
		// Reserve the failure budget before bcrypt runs.
		wkg.Use(basicAuthRateLimit)
		wkg.Use(middleware.BasicAuth(caldav.BasicAuth))
		wkg.Any("/caldav", caldav.PrincipalHandler)
		wkg.Any("/caldav/", caldav.PrincipalHandler)
		c := e.Group("/dav")
		c.Use(basicAuthRateLimit)
		registerCalDavRoutes(c)
	}

	// Feeds routes (Atom feed for user notifications)
	f := e.Group("/feeds")
	f.Use(basicAuthRateLimit)
	f.Use(middleware.BasicAuth(feeds.BasicAuth))
	f.GET("/notifications.atom", feeds.NotificationsAtomFeed)

	// healthcheck
	e.GET("/health", HealthcheckHandler)

	setupStaticFrontendFilesHandler(e)

	// CORS
	if config.CorsEnable.GetBool() {
		allowedOrigins := config.CorsOrigins.GetStringSlice()
		log.Infof("CORS enabled with origins: %s", strings.Join(allowedOrigins, ", "))

		// Echo v5 CORS middleware is stricter and doesn't accept wildcards in ports like "http://127.0.0.1:*"
		// We use UnsafeAllowOriginFunc to handle these patterns for backwards compatibility
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: []string{}, // Empty because we use UnsafeAllowOriginFunc
			UnsafeAllowOriginFunc: func(_ *echo.Context, origin string) (string, bool, error) {
				return matchCORSOrigin(origin, allowedOrigins)
			},
			AllowCredentials: true,
			MaxAge:           config.CorsMaxAge.GetInt(),
			Skipper: func(context *echo.Context) bool {
				// Since it is not possible to register this middleware just for the api group,
				// we just disable it when for caldav requests.
				// Caldav requires OPTIONS requests to be answered in a specific manner,
				// not doing this would break the caldav implementation.
				// Feed readers are server-side and don't need CORS either.
				p := context.Path()
				return strings.HasPrefix(p, "/dav") || strings.HasPrefix(p, "/feeds")
			},
		}))
	}

	// API Routes. The Huma-backed v2 contract is the only public API surface.
	a2 := e.Group("/api/v2")
	// Share the BasicAuth failure budget with CalDAV and feeds.
	a2.Use(pathScoped(func(p string) bool { return p == "/api/v2/notifications.atom" }, basicAuthRateLimit))
	registerAPIRoutesV2(e, a2, noAuthRateLimit, refreshRateLimit)
	setupMetrics(e)
	setupPprof(e)

	// Collect routes for API token permissions
	// In Echo v5, we collect routes after registration using e.Router().Routes()
	collectRoutesForAPITokens(e)
}

// unauthenticatedAPIPaths contains paths that don't require JWT authentication
var unauthenticatedAPIPaths = map[string]bool{
	"/api/v2/openapi.json":              true,
	"/api/v2/openapi.yaml":              true,
	"/api/v2/openapi-3.0.json":          true,
	"/api/v2/openapi-3.0.yaml":          true,
	"/api/v2/docs":                      true,
	"/api/v2/docs/scalar.standalone.js": true,
	"/api/v2/schemas/:schema":           true,
	"/api/v2/info":                      true,

	"/api/v2/user/confirm":                   true,
	"/api/v2/oauth/token":                    true,
	"/api/v2/login":                          true,
	auth.RefreshTokenPath:                    true,
	"/api/v2/auth/openid/:provider/callback": true,

	// Public infra healthcheck (a Huma op that opts out of the global auth).
	"/api/v2/health": true,

	// Atom feed (a Huma op) authenticates itself with HTTP Basic auth (a
	// feeds-scoped API token), like its /feeds counterpart, not a JWT.
	"/api/v2/notifications.atom": true,

	// WebSocket upgrade (a raw echo route — OpenAPI can't model WebSockets);
	// it authenticates via its first message, so the upgrade needs no JWT.
	"/api/v2/ws": true,
}

// collectRoutesForAPITokens collects all routes for API token permission checking.
// In Echo v5, OnAddRouteHandler was removed, so we collect routes after registration.
func collectRoutesForAPITokens(e *echo.Echo) {
	routeList := e.Router().Routes()
	log.Debugf("Collecting %d routes for API token usage", len(routeList))
	for _, route := range routeList {
		// Only process the public API contract.
		if !strings.HasPrefix(route.Path, "/api/v2") {
			continue
		}

		// Check if this route requires JWT authentication
		requiresJWT := !unauthenticatedAPIPaths[route.Path]

		models.CollectRoutesForAPITokenUsage(route, requiresJWT)
	}
}

// noStoreCacheControl returns middleware that sets `Cache-Control: no-store`
// on all responses. Without this, browsers may heuristically cache JSON
// responses, so changes made through the API might not appear until a hard
// refresh. Applied to the public API contract.
func noStoreCacheControl() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Set("Cache-Control", "no-store")
			return next(c)
		}
	}
}

// match receives the matched echo route template, not the request URL.
// v2 can't use an Echo sub-group here: that would split the Huma API and drop
// the scoped ops from the OpenAPI spec.
func pathScoped(match func(string) bool, mw echo.MiddlewareFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		scoped := mw(next)
		return func(c *echo.Context) error {
			if match(c.Path()) {
				return scoped(c)
			}
			return next(c)
		}
	}
}

type pathSet map[string]bool

func (s pathSet) has(p string) bool { return s[p] }

// Panics on a path that isn't JWT-exempt: such an entry is a typo, and a typo
// here silently means no rate limit at all.
func unauthenticatedPathSet(paths ...string) pathSet {
	s := make(pathSet, len(paths))
	for _, p := range paths {
		if !unauthenticatedAPIPaths[p] {
			panic("rate limited path " + p + " is not in unauthenticatedAPIPaths")
		}
		s[p] = true
	}
	return s
}

// Credential endpoints only, never the docs/info/health ones.
var v2CredentialPaths = unauthenticatedPathSet(
	"/api/v2/user/confirm",
	"/api/v2/login",
	"/api/v2/auth/openid/:provider/callback",
)

var v2SessionRenewalPaths = unauthenticatedPathSet(
	auth.RefreshTokenPath,
	"/api/v2/oauth/token",
)

const v2AdminPathPrefix = "/api/v2/admin"

// gateV2AdminRoutes hides admin endpoints unless the instance is licensed and
// the current user is an instance administrator.
func gateV2AdminRoutes() echo.MiddlewareFunc {
	feature := RequireFeature(license.FeatureAdminPanel)
	admin := RequireInstanceAdmin()
	return pathScoped(
		func(p string) bool { return strings.HasPrefix(p, v2AdminPathPrefix) },
		func(next echo.HandlerFunc) echo.HandlerFunc { return feature(admin(next)) },
	)
}

// registerAPIRoutesV2 wires the /api/v2 Echo group. Token middleware is
// attached before any route so Huma's spec and Scalar docs share the
// resource handlers' stack; unauthenticatedAPIPaths keeps them public.
func registerAPIRoutesV2(e *echo.Echo, a *echo.Group, noAuthRateLimit, refreshRateLimit echo.MiddlewareFunc) {
	a.Use(noStoreCacheControl())
	a.Use(SetupTokenMiddleware())
	a.Use(pathScoped(v2SessionRenewalPaths.has, refreshRateLimit))
	a.Use(pathScoped(v2CredentialPaths.has, noAuthRateLimit))
	// Rate limiting and route metrics apply to resource endpoints too.
	setupRateLimit(a, config.RateLimitKind.GetString())
	setupMetricsMiddleware(a)
	// Must come after rate limiting: the gate does a per-request admin DB read,
	// so an unauthenticated flood to /api/v2/admin/* would otherwise be unbounded.
	a.Use(gateV2AdminRoutes())

	api := apiv2.NewAPI(e, a)

	// Scalar docs UI — embedded, no CDN. See pkg/routes/api/v2/docs.go.
	a.GET("/docs", apiv2.ScalarUI)
	a.GET("/docs/scalar.standalone.js", apiv2.ScalarJS)

	// WebSockets can't be modeled in OpenAPI and Huma has no WS support, so the
	// upgrade endpoint stays a raw echo route (outside the Huma spec). It
	// authenticates via its first message, so unauthenticatedAPIPaths exempts it
	// from the group's JWT middleware. Health and the Atom feed are Huma ops and
	// self-register via init()/RegisterAll.
	a.GET("/ws", ws.UpgradeHandler, noAuthRateLimit)

	// Resources self-register via init(); RegisterAll runs them all + AutoPatch.
	apiv2.RegisterAll(api)
}

func registerCalDavRoutes(c *echo.Group) {

	// Basic auth middleware
	c.Use(middleware.BasicAuth(caldav.BasicAuth))

	// THIS is the entry point for caldav clients, otherwise projects will show up double
	c.Any("", caldav.EntryHandler)
	c.Any("/", caldav.EntryHandler)
	c.Any("/principals/*", caldav.PrincipalHandler)
	c.Any("/principals/*/", caldav.PrincipalHandler)
	c.Any("/projects", caldav.ProjectHandler)
	c.Any("/projects/", caldav.ProjectHandler)
	c.Any("/projects/:project", caldav.ProjectHandler)
	c.Any("/projects/:project/", caldav.ProjectHandler)
	c.Any("/projects/:project/:task", caldav.TaskHandler) // Mostly used for editing
}

// Level by status so log.httplevel can filter successful requests out
// while keeping failed ones.
func httpLogLevel(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
