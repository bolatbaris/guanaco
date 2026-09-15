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
	"fmt"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

// TaskDelegation is the action payload for assigning a task to an external
// person represented only by a full-name string.
type TaskDelegation struct {
	TaskID        int64  `json:"-" param:"projecttask" readOnly:"true" doc:"The numeric id of the task to delegate, taken from the URL."`
	DelegateeName string `json:"delegatee_name" required:"true" valid:"required,runelength(1|250)" minLength:"1" maxLength:"250" doc:"The external delegate's full name."`

	web.CRUDable    `xorm:"-" json:"-"`
	web.Permissions `xorm:"-" json:"-"`
}

// CanCreate allows only a real user with task update permission to set a
// delegation.
func (td *TaskDelegation) CanCreate(s *xorm.Session, a web.Auth) (bool, error) {
	if _, err := delegationUserFromAuth(a); err != nil {
		return false, err
	}
	if _, _, err := canonicalizeDelegationName(td.DelegateeName); err != nil {
		return false, err
	}

	return (&Task{ID: td.TaskID}).CanUpdate(s, a)
}

// CanDelete allows only a real user with task update permission to clear a
// delegation.
func (td *TaskDelegation) CanDelete(s *xorm.Session, a web.Auth) (bool, error) {
	if _, err := delegationUserFromAuth(a); err != nil {
		return false, err
	}

	return (&Task{ID: td.TaskID}).CanUpdate(s, a)
}

// Create sets the external delegate and records the canonical name in the
// authenticated user's history. All writes use the handler-owned session.
func (td *TaskDelegation) Create(s *xorm.Session, a web.Auth) (err error) {
	delegator, err := delegationUserFromAuth(a)
	if err != nil {
		return err
	}

	canonical, normalized, err := canonicalizeDelegationName(td.DelegateeName)
	if err != nil {
		return err
	}

	task, err := lockTaskForDelegation(s, td.TaskID)
	if err != nil {
		return err
	}

	if normalizedStoredDelegationName(task.DelegatedTo) == normalized {
		td.DelegateeName = task.DelegatedTo
		return nil
	}

	if err := lockDelegationOwner(s, delegator.ID); err != nil {
		return err
	}

	canonical, err = incrementDelegationName(s, delegator.ID, canonical, normalized)
	if err != nil {
		return err
	}

	if _, err := s.ID(task.ID).Cols("delegated_to").Update(&Task{DelegatedTo: canonical}); err != nil {
		return fmt.Errorf("could not set task delegation: %w", err)
	}
	task.DelegatedTo = canonical

	if err := updateTaskLastUpdated(s, &Task{ID: task.ID}); err != nil {
		return fmt.Errorf("could not update delegated task timestamp: %w", err)
	}
	if err := updateProjectLastUpdated(s, &Project{ID: task.ProjectID}); err != nil {
		return fmt.Errorf("could not update delegated project timestamp: %w", err)
	}

	td.DelegateeName = canonical
	return triggerTaskUpdatedEventForTaskID(s, a, task.ID)
}

// Delete clears the external delegation and restores the authenticated user as
// the task owner through its created_by_id value.
func (td *TaskDelegation) Delete(s *xorm.Session, a web.Auth) (err error) {
	if _, err := delegationUserFromAuth(a); err != nil {
		return err
	}

	task, err := lockTaskForDelegation(s, td.TaskID)
	if err != nil {
		return err
	}

	if task.DelegatedTo == "" {
		return nil
	}

	if _, err := s.ID(task.ID).Cols("delegated_to").Update(&Task{DelegatedTo: ""}); err != nil {
		return fmt.Errorf("could not clear task delegation: %w", err)
	}
	task.DelegatedTo = ""

	if err := updateTaskLastUpdated(s, &Task{ID: task.ID}); err != nil {
		return fmt.Errorf("could not update restored task timestamp: %w", err)
	}

	if err := updateProjectLastUpdated(s, &Project{ID: task.ProjectID}); err != nil {
		return fmt.Errorf("could not update restored project timestamp: %w", err)
	}

	return triggerTaskUpdatedEventForTaskID(s, a, task.ID)
}

func delegationUserFromAuth(a web.Auth) (*user.User, error) {
	delegator, ok := a.(*user.User)
	if !ok || delegator == nil {
		return nil, &user.ErrMustNotBeLinkShare{}
	}
	return delegator, nil
}

func lockTaskForDelegation(s *xorm.Session, taskID int64) (*Task, error) {
	if taskID < 1 {
		return nil, ErrTaskDoesNotExist{ID: taskID}
	}

	task := &Task{ID: taskID}
	query := s.ID(taskID)
	if db.Type() != schemas.SQLITE {
		query = query.ForUpdate()
	}
	exists, err := query.Get(task)
	if err != nil {
		return nil, fmt.Errorf("could not lock task for delegation: %w", err)
	}
	if !exists {
		return nil, ErrTaskDoesNotExist{ID: taskID}
	}
	return task, nil
}

func lockDelegationOwner(s *xorm.Session, ownerID int64) error {
	owner := &user.User{}
	query := s.Where("id = ?", ownerID)
	if db.Type() != schemas.SQLITE {
		query = query.ForUpdate()
	}
	exists, err := query.Get(owner)
	if err != nil {
		return fmt.Errorf("could not lock delegation owner: %w", err)
	}
	if !exists {
		return user.ErrUserDoesNotExist{UserID: ownerID}
	}
	return nil
}

func incrementDelegationName(s *xorm.Session, ownerID int64, canonical, normalized string) (string, error) {
	name := &DelegationName{}
	query := s.Where("owner_id = ? AND normalized_name = ?", ownerID, normalized)
	if db.Type() != schemas.SQLITE {
		query = query.ForUpdate()
	}
	exists, err := query.Get(name)
	if err != nil {
		return "", fmt.Errorf("could not find delegation name: %w", err)
	}

	if exists {
		name.UsageCount++
		if _, err := s.ID(name.ID).Cols("usage_count").Update(&DelegationName{UsageCount: name.UsageCount}); err != nil {
			return "", fmt.Errorf("could not increment delegation name usage: %w", err)
		}
		return name.Name, nil
	}

	name = &DelegationName{
		OwnerID:        ownerID,
		Name:           canonical,
		NormalizedName: normalized,
		UsageCount:     1,
	}
	if _, err := s.Insert(name); err != nil {
		return "", fmt.Errorf("could not store delegation name: %w", err)
	}
	return name.Name, nil
}
