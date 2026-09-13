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

package initialize

import (
	"fmt"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/user"
	"xorm.io/xorm"
)

type singleUserConfig struct {
	username string
	password string
	email    string
	name     string
}

func ensureSingleUser() error {
	if !config.AuthSingleUserEnabled.GetBool() {
		return nil
	}

	cfg, err := getSingleUserConfig()
	if err != nil {
		return err
	}

	s := db.NewSession()
	defer s.Close()
	defer events.CleanupPending(s)

	singleUser, err := getOrCreateSingleUser(s, cfg)
	if err != nil {
		return err
	}
	if err := finalizeSingleUser(s, singleUser); err != nil {
		return err
	}

	if err := s.Commit(); err != nil {
		return fmt.Errorf("could not persist configured single user: %w", err)
	}

	return nil
}

func getSingleUserConfig() (*singleUserConfig, error) {
	cfg := &singleUserConfig{
		username: config.AuthSingleUserUsername.GetString(),
		password: config.AuthSingleUserPassword.GetString(),
		email:    config.AuthSingleUserEmail.GetString(),
		name:     config.AuthSingleUserName.GetString(),
	}

	if cfg.username == "" || cfg.password == "" || cfg.email == "" {
		return nil, fmt.Errorf("auth.singleuser.username, auth.singleuser.password and auth.singleuser.email are required when single-user mode is enabled")
	}
	if len([]byte(cfg.password)) > 72 {
		return nil, fmt.Errorf("auth.singleuser.password must be at most 72 bytes")
	}
	if !config.AuthLocalEnabled.GetBool() {
		return nil, fmt.Errorf("auth.local.enabled must be true when single-user mode is enabled")
	}
	if config.AuthLdapEnabled.GetBool() || config.AuthOpenIDEnabled.GetBool() {
		return nil, fmt.Errorf("external authentication must be disabled when single-user mode is enabled")
	}

	return cfg, nil
}

func getOrCreateSingleUser(s *xorm.Session, cfg *singleUserConfig) (*user.User, error) {
	singleUser, err := user.GetUserWithEmail(s, &user.User{Username: cfg.username})
	userExists := err == nil || user.IsErrUserStatusError(err)
	if err != nil && !userExists && !user.IsErrUserDoesNotExist(err) {
		return nil, fmt.Errorf("could not load configured single user: %w", err)
	}

	if userExists {
		if err := synchronizeSingleUser(s, singleUser, cfg); err != nil {
			return nil, err
		}
		return singleUser, nil
	}

	singleUser, err = models.RegisterUser(s, &user.User{
		Username: cfg.username,
		Password: cfg.password,
		Email:    cfg.email,
		Name:     cfg.name,
	})
	if err != nil {
		return nil, fmt.Errorf("could not provision configured single user: %w", err)
	}
	return singleUser, nil
}

func synchronizeSingleUser(s *xorm.Session, singleUser *user.User, cfg *singleUserConfig) error {
	if singleUser.Issuer != user.IssuerLocal || singleUser.IsBot() {
		return fmt.Errorf("configured single user must be a local human account")
	}

	if user.CheckUserPassword(singleUser, cfg.password) != nil {
		if err := user.UpdateUserPassword(s, singleUser, cfg.password); err != nil {
			return fmt.Errorf("could not update configured single user password: %w", err)
		}
		if err := models.DeleteAllUserSessions(s, singleUser.ID); err != nil {
			return fmt.Errorf("could not invalidate configured single user sessions: %w", err)
		}
	}

	if err := synchronizeSingleUserProfile(s, singleUser, cfg); err != nil {
		return err
	}
	if singleUser.Status != user.StatusActive {
		if err := user.SetUserStatus(s, singleUser, user.StatusActive); err != nil {
			return fmt.Errorf("could not activate configured single user: %w", err)
		}
	}
	return nil
}

func synchronizeSingleUserProfile(s *xorm.Session, singleUser *user.User, cfg *singleUserConfig) error {
	if singleUser.Email == cfg.email && (cfg.name == "" || singleUser.Name == cfg.name) {
		return nil
	}

	updates := &user.User{Email: cfg.email}
	columns := []string{"email"}
	if cfg.name != "" && singleUser.Name != cfg.name {
		updates.Name = cfg.name
		columns = append(columns, "name")
	}
	if _, err := s.ID(singleUser.ID).Cols(columns...).Update(updates); err != nil {
		return fmt.Errorf("could not synchronize configured single user profile: %w", err)
	}
	return nil
}

func finalizeSingleUser(s *xorm.Session, singleUser *user.User) error {
	if !singleUser.IsAdmin {
		if _, err := s.ID(singleUser.ID).Cols("is_admin").Update(&user.User{IsAdmin: true}); err != nil {
			return fmt.Errorf("could not grant configured single user administrator access: %w", err)
		}
	}
	if singleUser.DefaultProjectID == 0 {
		if err := models.CreateNewProjectForUser(s, singleUser); err != nil {
			return fmt.Errorf("could not create the configured single user's default project: %w", err)
		}
	}
	if _, err := s.ID(singleUser.ID).Cols("is_admin", "status").Update(&user.User{
		IsAdmin: true,
		Status:  user.StatusActive,
	}); err != nil {
		return fmt.Errorf("could not finalize configured single user: %w", err)
	}
	return nil
}
