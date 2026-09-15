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
	"time"

	"src.techknowlogick.com/xormigrate"
	"xorm.io/xorm"
)

type tasksDelegatedTo20260913204153 struct {
	DelegatedTo string `xorm:"varchar(250) null"`
}

func (tasksDelegatedTo20260913204153) TableName() string {
	return "tasks"
}

type DelegationName20260913204153 struct {
	ID             int64     `xorm:"bigint autoincr not null unique pk"`
	OwnerID        int64     `xorm:"bigint INDEX not null unique(delegation_name_owner_normalized)"`
	Name           string    `xorm:"varchar(250) not null"`
	NormalizedName string    `xorm:"varchar(250) not null unique(delegation_name_owner_normalized)"`
	UsageCount     int64     `xorm:"bigint not null default 0"`
	Created        time.Time `xorm:"created not null"`
	Updated        time.Time `xorm:"updated not null"`
}

func (DelegationName20260913204153) TableName() string {
	return "delegation_names"
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20260913204153",
		Description: "Add task delegation support",
		Migrate: func(tx *xorm.Engine) error {
			if err := partialSync(tx, tasksDelegatedTo20260913204153{}); err != nil {
				return fmt.Errorf("could not add delegated_to column to tasks: %w", err)
			}

			if err := tx.Sync(DelegationName20260913204153{}); err != nil { //nolint:forbidigo // brand-new table, nothing to drop
				return fmt.Errorf("could not create delegation_names table: %w", err)
			}

			return nil
		},
		Rollback: func(tx *xorm.Engine) error {
			if err := tx.DropTables(DelegationName20260913204153{}); err != nil {
				return fmt.Errorf("could not drop delegation_names table: %w", err)
			}

			if err := dropTableColum(tx, "tasks", "delegated_to"); err != nil {
				return fmt.Errorf("could not drop delegated_to column from tasks: %w", err)
			}

			return nil
		},
	})
}
