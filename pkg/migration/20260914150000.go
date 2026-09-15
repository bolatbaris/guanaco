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

package migration

import (
	"fmt"

	"src.techknowlogick.com/xormigrate"
	"xorm.io/xorm"
)

type taskAssignees20260914150000 struct{}

func (taskAssignees20260914150000) TableName() string {
	return "task_assignees"
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20260914150000",
		Description: "Remove task assignee storage",
		Migrate: func(tx *xorm.Engine) error {
			if err := tx.DropTables(taskAssignees20260914150000{}); err != nil {
				return fmt.Errorf("could not drop task_assignees table: %w", err)
			}
			return nil
		},
		Rollback: func(_ *xorm.Engine) error {
			return nil
		},
	})
}
