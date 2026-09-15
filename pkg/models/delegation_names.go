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
	"strings"
	"time"
	"unicode/utf8"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/web"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"xorm.io/builder"
	"xorm.io/xorm"
)

const maxDelegationNameLength = 250

var delegationNameFold = cases.Fold()

// DelegationName stores one canonical external delegate name in a user's
// delegation history.
type DelegationName struct {
	ID             int64     `xorm:"bigint autoincr not null unique pk" json:"id" readOnly:"true" doc:"The unique numeric id of this delegation name."`
	OwnerID        int64     `xorm:"bigint INDEX not null unique(delegation_name_owner_normalized)" json:"-" readOnly:"true" doc:"The user who owns this delegation name history."`
	Name           string    `xorm:"varchar(250) not null" json:"name" readOnly:"true" doc:"The first spelling stored for this delegate name."`
	NormalizedName string    `xorm:"varchar(250) not null unique(delegation_name_owner_normalized)" json:"-" readOnly:"true" doc:"The normalized value used to match delegate names."`
	UsageCount     int64     `xorm:"bigint not null default 0" json:"usage_count" readOnly:"true" doc:"The number of times this name has been used for a new delegation."`
	Created        time.Time `xorm:"created not null" json:"created" readOnly:"true" doc:"When this delegation name was first stored."`
	Updated        time.Time `xorm:"updated not null" json:"updated" readOnly:"true" doc:"When this delegation name was last used."`

	web.CRUDable    `xorm:"-" json:"-"`
	web.Permissions `xorm:"-" json:"-"`
}

// TableName returns the persisted delegation-name table.
func (*DelegationName) TableName() string {
	return "delegation_names"
}

// ReadAll returns the authenticated user's delegation history, ordered by
// usage so the most common choices are shown first.
func (dn *DelegationName) ReadAll(s *xorm.Session, a web.Auth, search string, page int, perPage int) (result interface{}, resultCount int, totalItems int64, err error) {
	if _, err := delegationUserFromAuth(a); err != nil {
		return nil, 0, 0, err
	}

	where := []builder.Cond{builder.Eq{"owner_id": a.GetID()}}
	if normalizedSearch := normalizeDelegationSearch(search); normalizedSearch != "" {
		where = append(where, db.ILIKE("normalized_name", normalizedSearch))
	}
	condition := builder.And(where...)

	totalItems, err = s.Where(condition).Count(&DelegationName{})
	if err != nil {
		return nil, 0, 0, fmt.Errorf("could not count delegation names: %w", err)
	}

	query := s.Where(condition).OrderBy("usage_count DESC", "name ASC")
	if limit, start := getLimitFromPageIndex(page, perPage); limit > 0 {
		query = query.Limit(limit, start)
	}

	names := []*DelegationName{}
	if err = query.Find(&names); err != nil {
		return nil, 0, 0, fmt.Errorf("could not list delegation names: %w", err)
	}

	return names, len(names), totalItems, nil
}

func normalizeDelegationSearch(search string) string {
	return normalizeDelegationName(search)
}

func canonicalizeDelegationName(name string) (canonical string, normalized string, err error) {
	canonical = strings.Join(strings.Fields(name), " ")
	if canonical == "" {
		return "", "", ErrInvalidData{Message: "The delegation name cannot be empty."}
	}
	if utf8.RuneCountInString(canonical) > maxDelegationNameLength {
		return "", "", ErrInvalidData{Message: "The delegation name must be at most 250 characters long."}
	}

	return canonical, normalizeDelegationName(canonical), nil
}

func normalizedStoredDelegationName(name string) string {
	return normalizeDelegationName(name)
}

func normalizeDelegationName(name string) string {
	collapsed := strings.Join(strings.Fields(name), " ")
	return norm.NFC.String(delegationNameFold.String(norm.NFC.String(collapsed)))
}
