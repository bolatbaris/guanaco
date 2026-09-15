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

package models

import (
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

// ProjectUser represents a project <-> user relation
type ProjectUser struct {
	// The unique, numeric id of this project <-> user relation.
	ID int64 `xorm:"bigint autoincr not null unique pk" json:"id" readOnly:"true" doc:"The unique, numeric id of this project <-> user relation."`
	// The username.
	Username string `xorm:"-" json:"username" param:"user" doc:"The username of the user to share with. On update and delete this comes from the URL path, not the body."`
	// Used internally to reference the user
	UserID int64 `xorm:"bigint not null INDEX" json:"-"`
	// The project id.
	ProjectID int64 `xorm:"bigint not null INDEX" json:"-" param:"project"`
	// The permission this user has. 0 = Read only, 1 = Read & Write, 2 = Admin. See the docs for more details.
	Permission Permission `xorm:"bigint INDEX not null default 0" json:"permission" valid:"length(0|2)" maximum:"2" default:"0" doc:"The permission this user has on the project. 0 = Read only, 1 = Read & Write, 2 = Admin."`

	// A timestamp when this relation was created. You cannot change this value.
	Created time.Time `xorm:"created not null" json:"created" readOnly:"true" doc:"A timestamp when this relation was created. You cannot change this value."`
	// A timestamp when this relation was last updated. You cannot change this value.
	Updated time.Time `xorm:"updated not null" json:"updated" readOnly:"true" doc:"A timestamp when this relation was last updated. You cannot change this value."`

	web.CRUDable    `xorm:"-" json:"-"`
	web.Permissions `xorm:"-" json:"-"`
}

// TableName is the table name for ProjectUser
func (*ProjectUser) TableName() string {
	return "users_projects"
}

// UserWithPermission represents a user in combination with the permission it can have on a project
type UserWithPermission struct {
	user.User  `xorm:"extends"`
	Permission Permission `json:"permission" readOnly:"true" doc:"The permission this user has on the project. 0 = Read only, 1 = Read & Write, 2 = Admin."`
}

// Create creates a new project <-> user relation.
func (lu *ProjectUser) Create(s *xorm.Session, a web.Auth) (err error) {

	// Check if the permission is valid
	if err := lu.Permission.isValid(); err != nil {
		return err
	}

	// Check if the project exists
	l, err := GetProjectSimpleByID(s, lu.ProjectID)
	if err != nil {
		return
	}

	// Check if the user exists
	u, err := user.GetUserByUsername(s, lu.Username)
	if err != nil {
		return err
	}
	lu.UserID = u.ID

	// Check if the user already has access or is owner of that project
	// Access is intentionally limited to direct user shares.
	if l.OwnerID == lu.UserID {
		return ErrUserAlreadyHasAccess{UserID: lu.UserID, ProjectID: lu.ProjectID}
	}

	exist, err := s.Where("project_id = ? AND user_id = ?", lu.ProjectID, lu.UserID).Get(&ProjectUser{})
	if err != nil {
		return
	}
	if exist {
		return ErrUserAlreadyHasAccess{UserID: lu.UserID, ProjectID: lu.ProjectID}
	}

	lu.ID = 0
	_, err = s.Insert(lu)
	if err != nil {
		return err
	}

	events.DispatchOnCommit(s, &ProjectSharedWithUserEvent{
		Project: l,
		User:    u,
		Doer:    doerFromAuth(s, a),
	})

	err = updateProjectLastUpdated(s, l)
	return
}

// Delete deletes a project <-> user relation.
func (lu *ProjectUser) Delete(s *xorm.Session, _ web.Auth) (err error) {

	// Check if the user exists
	u, err := user.GetUserByUsername(s, lu.Username)
	if err != nil && !user.IsErrUserStatusError(err) {
		return
	}
	lu.UserID = u.ID

	// Check if the user has access to the project
	has, err := s.
		Where("user_id = ? AND project_id = ?", lu.UserID, lu.ProjectID).
		Get(&ProjectUser{})
	if err != nil {
		return
	}
	if !has {
		return ErrUserDoesNotHaveAccessToProject{ProjectID: lu.ProjectID, UserID: lu.UserID}
	}

	_, err = s.
		Where("user_id = ? AND project_id = ?", lu.UserID, lu.ProjectID).
		Delete(&ProjectUser{})
	if err != nil {
		return err
	}

	err = updateProjectLastUpdated(s, &Project{ID: lu.ProjectID})
	return
}

// ReadAll gets all users who have access to a project.
func (lu *ProjectUser) ReadAll(s *xorm.Session, a web.Auth, search string, page int, perPage int) (result interface{}, resultCount int, numberOfTotalItems int64, err error) {
	// Link shares must not see the user directory of a project
	if _, is := a.(*LinkSharing); is {
		return nil, 0, 0, &user.ErrMustNotBeLinkShare{}
	}

	// Check if the user has access to the project
	l := &Project{ID: lu.ProjectID}
	canRead, _, err := l.CanRead(s, a)
	if err != nil {
		return nil, 0, 0, err
	}
	if !canRead {
		return nil, 0, 0, ErrNeedToHaveProjectReadAccess{UserID: a.GetID(), ProjectID: lu.ProjectID}
	}

	limit, start := getLimitFromPageIndex(page, perPage)

	// Get all users
	all := []*UserWithPermission{}
	query := s.
		Join("INNER", "users_projects", "user_id = users.id").
		Where("users_projects.project_id = ?", lu.ProjectID).
		Where(db.ILIKE("users.username", search))
	if limit > 0 {
		query = query.Limit(limit, start)
	}
	err = query.Find(&all)
	if err != nil {
		return nil, 0, 0, err
	}

	// Obfuscate all user emails
	for _, u := range all {
		u.Email = ""
	}

	numberOfTotalItems, err = s.
		Join("INNER", "users_projects", "user_id = users.id").
		Where("users_projects.project_id = ?", lu.ProjectID).
		Where("users.username LIKE ?", "%"+search+"%").
		Count(&UserWithPermission{})

	return all, len(all), numberOfTotalItems, err
}

// Update updates a user <-> project relation.
func (lu *ProjectUser) Update(s *xorm.Session, _ web.Auth) (err error) {

	// Check if the permission is valid
	if err := lu.Permission.isValid(); err != nil {
		return err
	}

	// Check if the user exists
	u, err := user.GetUserByUsername(s, lu.Username)
	if err != nil && !user.IsErrUserStatusError(err) {
		return err
	}
	lu.UserID = u.ID

	_, err = s.
		Where("project_id = ? AND user_id = ?", lu.ProjectID, lu.UserID).
		Cols("permission").
		Update(lu)
	if err != nil {
		return err
	}

	err = updateProjectLastUpdated(s, &Project{ID: lu.ProjectID})
	return
}
