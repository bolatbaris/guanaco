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

package shared

import (
	"context"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/modules/auth/ldap"
	"code.vikunja.io/api/pkg/modules/auth/openid"
	"code.vikunja.io/api/pkg/modules/keyvalue"
	"code.vikunja.io/api/pkg/user"

	"xorm.io/xorm"
)

// AuthenticateUserCredentials verifies a login against local (and, if configured,
// LDAP) credentials and enforces the account-status and TOTP gates, returning the
// authenticated user on success. It is the transport-agnostic core of the login
// flow; the caller issues the token and sets the cookie. The
// returned errors carry their own HTTP semantics (wrong credentials, disabled
// account, missing/invalid TOTP) so both APIs surface them identically.
func AuthenticateUserCredentials(ctx context.Context, login *user.Login) (*user.User, error) {
	s := db.NewSession()
	defer s.Close()
	// Discards events queued during a rolled-back transaction (e.g. LDAP user
	// creation); a no-op once DispatchPending has run.
	defer events.CleanupPending(s)

	u, err := resolveLoginUser(ctx, s, login)
	if err != nil {
		_ = s.Rollback()
		return nil, err
	}

	if u.Status == user.StatusDisabled {
		_ = s.Rollback()
		return nil, &user.ErrAccountDisabled{UserID: u.ID}
	}
	if u.Status == user.StatusAccountLocked {
		_ = s.Rollback()
		return nil, &user.ErrAccountLocked{UserID: u.ID}
	}

	if err := enforceLoginTOTP(s, u, login.TOTPPasscode); err != nil {
		return nil, err
	}

	if err := keyvalue.Del(u.GetFailedTOTPAttemptsKey()); err != nil {
		return nil, err
	}
	if err := keyvalue.Del(u.GetFailedPasswordAttemptsKey()); err != nil {
		return nil, err
	}

	if err := s.Commit(); err != nil {
		_ = s.Rollback()
		return nil, err
	}

	events.DispatchPending(ctx, s)

	return u, nil
}

// resolveLoginUser authenticates the configured local account in single-user
// mode. Otherwise it keeps the existing LDAP/local login order. Bots are
// rejected before bcrypt runs because they have no password hash.
func resolveLoginUser(ctx context.Context, s *xorm.Session, login *user.Login) (*user.User, error) {
	if config.AuthSingleUserEnabled.GetBool() {
		return user.CheckUserCredentials(ctx, s, login)
	}

	if config.AuthLdapEnabled.GetBool() {
		u, err := ldap.AuthenticateUserInLDAP(s, login.Username, login.Password, config.AuthLdapAvatarSyncAttribute.GetString())
		if err != nil && !user.IsErrWrongUsernameOrPassword(err) {
			return nil, err
		}
		if u != nil {
			return u, nil
		}
	}

	existingUser, lookupErr := user.GetUserByUsername(s, login.Username)
	if lookupErr == nil && existingUser.IsBot() {
		return nil, &user.ErrAccountIsBot{UserID: existingUser.ID}
	}

	return user.CheckUserCredentials(ctx, s, login)
}

// enforceLoginTOTP runs the TOTP gate for users who have it enabled: a missing
// passcode is rejected, and a wrong one trips the failed-attempt
// lockout via HandleFailedTOTPAuth. The session is rolled back before
// HandleFailedTOTPAuth so its dedicated session can acquire a write lock on
// SQLite shared-cache (the lockout write is decoupled from this transaction —
// see GHSA-fgfv-pv97-6cmj).
func enforceLoginTOTP(s *xorm.Session, u *user.User, passcode string) error {
	totpEnabled, err := user.TOTPEnabledForUser(s, u)
	if err != nil {
		_ = s.Rollback()
		return err
	}
	if !totpEnabled {
		return nil
	}

	if passcode == "" {
		_ = s.Rollback()
		return user.ErrInvalidTOTPPasscode{}
	}

	_, err = user.ValidateTOTPPasscode(s, &user.TOTPPasscode{User: u, Passcode: passcode})
	if err != nil {
		_ = s.Rollback()
		if user.IsErrInvalidTOTPPasscode(err) {
			user.HandleFailedTOTPAuth(u)
		}
		return err
	}

	return nil
}

// DeleteSession removes the session with the given id, logging the user out
// server-side. An empty sid is a no-op (the token carried no session, e.g. an
// API token or a link share); the caller is
// responsible for clearing the refresh cookie.
func DeleteSession(sid string) error {
	_, err := LogoutSession(sid)
	return err
}

// LogoutSession deletes the session and returns its OIDC RP-Initiated Logout URL
// for the frontend to redirect to (empty for non-OIDC sessions or when no logout
// endpoint is configured). An empty sid is a no-op. The caller clears the refresh
// cookie.
func LogoutSession(sid string) (endSessionURL string, err error) {
	if sid == "" {
		return "", nil
	}

	s := db.NewSession()
	defer s.Close()

	// Read before deleting so the stored id_token survives for the logout URL.
	// A missing session just means there is nothing to log out.
	session, err := models.GetSessionByID(s, sid)
	if err != nil && !models.IsErrSessionNotFound(err) {
		_ = s.Rollback()
		return "", err
	}
	if session != nil && session.OIDCProviderKey != "" {
		url, buildErr := openid.BuildEndSessionURL(session.OIDCProviderKey, &models.SessionOIDCData{
			IDToken:     session.OIDCIDToken,
			ProviderKey: session.OIDCProviderKey,
		})
		if buildErr != nil {
			// A failed URL build must not block logout; the session is still deleted below.
			log.Errorf("Could not build OIDC end-session URL for session %s: %v", sid, buildErr)
		} else {
			endSessionURL = url
		}
	}

	if _, err := s.Where("id = ?", sid).Delete(&models.Session{}); err != nil {
		_ = s.Rollback()
		return "", err
	}

	if err := s.Commit(); err != nil {
		_ = s.Rollback()
		return "", err
	}

	return endSessionURL, nil
}

// ConfirmEmail confirms an account's email from the token sent to it.
func ConfirmEmail(confirm *user.EmailConfirm) error {
	s := db.NewSession()
	defer s.Close()

	if err := user.ConfirmEmail(s, confirm); err != nil {
		_ = s.Rollback()
		return err
	}

	return s.Commit()
}
