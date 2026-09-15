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

	"code.vikunja.io/api/pkg/models"

	"src.techknowlogick.com/xormigrate"
	"xorm.io/xorm"
)

// These numeric values are frozen because the corresponding model constants
// are intentionally removed after this migration.
const (
	taskFavoriteKindLegacy20260914120000       = 1
	taskSubscriptionEntityLegacy20260914120000 = 3
)

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20260914120000",
		Description: "Remove task-specific favorites, subscriptions and fields",
		Migrate: func(tx *xorm.Engine) error {
			if _, err := tx.Where("kind = ?", taskFavoriteKindLegacy20260914120000).Delete(&models.Favorite{}); err != nil {
				return fmt.Errorf("could not remove task favorite rows: %w", err)
			}

			if _, err := tx.Where("entity_type = ?", taskSubscriptionEntityLegacy20260914120000).Delete(&models.Subscription{}); err != nil {
				return fmt.Errorf("could not remove task subscription rows: %w", err)
			}

			for _, column := range []string{"priority", "hex_color", "percent_done"} {
				exists, err := columnExists(tx, "tasks", column)
				if err != nil {
					return fmt.Errorf("could not check for tasks.%s: %w", column, err)
				}
				if !exists {
					continue
				}
				if err := dropTableColum(tx, "tasks", column); err != nil {
					return fmt.Errorf("could not remove tasks.%s: %w", column, err)
				}
			}

			return nil
		},
		Rollback: func(_ *xorm.Engine) error {
			return nil
		},
	})
}
