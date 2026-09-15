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
	"regexp"
	"strings"

	"github.com/ganigeorgiev/fexpr"
	"src.techknowlogick.com/xormigrate"
	"xorm.io/builder"
	"xorm.io/xorm"
)

type taskCollectionRemoveAssigneeFilters20260914181407 struct {
	Search             string   `query:"q" json:"q"`
	SortBy             []string `query:"sort_by" json:"sort_by"`
	OrderBy            []string `query:"order_by" json:"order_by"`
	Filter             string   `query:"filter" json:"filter"`
	FilterIncludeNulls bool     `query:"filter_include_nulls" json:"filter_include_nulls"`
}

type savedFilterRemoveAssigneeFilters20260914181407 struct {
	ID      int64                                              `xorm:"autoincr not null unique pk" json:"id" param:"filter"`
	Filters *taskCollectionRemoveAssigneeFilters20260914181407 `xorm:"JSON not null" json:"filters"`
}

func (savedFilterRemoveAssigneeFilters20260914181407) TableName() string {
	return "saved_filters"
}

func quotedRunEnd20260914181407(filter string, start int) int {
	quote := filter[start]
	for i := start + 1; i < len(filter); i++ {
		switch filter[i] {
		case '\\':
			i++
		case quote:
			return i + 1
		}
	}
	return -1
}

func replaceFilterOperators20260914181407(filter string) string {
	operators := []struct {
		operator string
		sigil    string
	}{
		{" not in ", " ?!= "},
		{" in ", " ?= "},
		{" like ", " ~ "},
	}

	var out strings.Builder
	for i := 0; i < len(filter); {
		if c := filter[i]; c == '\'' || c == '"' {
			if end := quotedRunEnd20260914181407(filter, i); end > 0 {
				out.WriteString(filter[i:end])
				i = end
				continue
			}
		}

		matched := false
		for _, operator := range operators {
			if strings.HasPrefix(filter[i:], operator.operator) {
				out.WriteString(operator.sigil)
				i += len(operator.operator)
				matched = true
				break
			}
		}
		if !matched {
			out.WriteByte(filter[i])
			i++
		}
	}
	return out.String()
}

// Older saved filters use human-readable operators that fexpr does not parse directly.
func prepareFilterForParsing20260914181407(filter string) string {
	filter = replaceFilterOperators20260914181407(filter)
	re := regexp.MustCompile(`(\w+)\s*(>=|<=|!=|~|\?=|\?!=|=|>|<)\s*([^&|()]+)`)
	return re.ReplaceAllStringFunc(filter, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}

		value := strings.TrimSpace(parts[3])
		if (strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) ||
			(strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) {
			return parts[1] + " " + parts[2] + " " + value
		}

		return parts[1] + " " + parts[2] + " '" + strings.ReplaceAll(value, "'", "\\'") + "'"
	})
}

func removeLegacyAssigneeFilters(groups []fexpr.ExprGroup) (cleaned []fexpr.ExprGroup, changed bool) {
	cleaned = make([]fexpr.ExprGroup, 0, len(groups))

	for _, group := range groups {
		switch item := group.Item.(type) {
		case fexpr.Expr:
			if strings.EqualFold(item.Left.Literal, "assignees") {
				changed = true
				continue
			}
		case []fexpr.ExprGroup:
			var nestedChanged bool
			item, nestedChanged = removeLegacyAssigneeFilters(item)
			if nestedChanged {
				changed = true
				if len(item) == 0 {
					continue
				}
				group.Item = item
			}
		}

		cleaned = append(cleaned, group)
	}

	return cleaned, changed
}

// Rebuild from the parsed expression so a legacy field inside a quoted value is not altered.
func formatFilterGroups(groups []fexpr.ExprGroup) string {
	var filter strings.Builder

	for i, group := range groups {
		if i > 0 {
			filter.WriteByte(' ')
			filter.WriteString(string(group.Join))
			filter.WriteByte(' ')
		}

		switch item := group.Item.(type) {
		case fexpr.Expr:
			filter.WriteString(item.Left.Literal)
			filter.WriteByte(' ')
			filter.WriteString(string(item.Op))
			filter.WriteByte(' ')
			filter.WriteString(item.Right.Literal)
		case []fexpr.ExprGroup:
			filter.WriteByte('(')
			filter.WriteString(formatFilterGroups(item))
			filter.WriteByte(')')
		}
	}

	return filter.String()
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20260914181407",
		Description: "Remove obsolete assignee conditions from saved filters",
		Migrate: func(tx *xorm.Engine) error {
			filters := []*savedFilterRemoveAssigneeFilters20260914181407{}
			if err := tx.Find(&filters); err != nil {
				return fmt.Errorf("could not load saved filters: %w", err)
			}

			for _, filter := range filters {
				if filter.Filters == nil || !strings.Contains(strings.ToLower(filter.Filters.Filter), "assignees") {
					continue
				}

				parsed, err := fexpr.Parse(filter.Filters.Filter)
				if err != nil {
					parsed, err = fexpr.Parse(prepareFilterForParsing20260914181407(filter.Filters.Filter))
				}
				if err != nil {
					return fmt.Errorf("could not parse saved filter %d: %w", filter.ID, err)
				}

				cleaned, changed := removeLegacyAssigneeFilters(parsed)
				if !changed {
					continue
				}

				filter.Filters.Filter = formatFilterGroups(cleaned)
				if _, err := tx.Where(builder.Eq{"id": filter.ID}).Cols("filters").Update(filter); err != nil {
					return fmt.Errorf("could not update saved filter %d: %w", filter.ID, err)
				}
			}

			return nil
		},
		Rollback: func(_ *xorm.Engine) error {
			return nil
		},
	})
}
