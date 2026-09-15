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

package caldav

import (
	"errors"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/utils"

	ics "github.com/arran4/golang-ical"
)

func GetCaldavTodosForTasks(project *models.ProjectWithTasksAndBuckets, projectTasks []*models.TaskWithComments) string {

	// Make caldav todos from Vikunja todos
	var caldavtodos []*Todo
	for _, t := range projectTasks {

		duration := t.EndDate.Sub(t.StartDate)
		var categories []string
		for _, label := range t.Labels {
			categories = append(categories, label.Title)
		}
		var alarms []Alarm
		for _, reminder := range t.Reminders {
			alarms = append(alarms, Alarm{
				Time:       reminder.Reminder,
				Duration:   time.Duration(reminder.RelativePeriod) * time.Second,
				RelativeTo: reminder.RelativeTo,
			})
		}

		var relations []Relation
		for reltype, tasks := range t.RelatedTasks {
			for _, r := range tasks {
				relations = append(relations, Relation{
					Type: reltype,
					UID:  r.UID,
				})
			}
		}

		caldavtodos = append(caldavtodos, &Todo{
			Timestamp:   t.Updated,
			UID:         t.UID,
			Summary:     t.Title,
			Description: t.Description,
			Done:        t.Done,
			Completed:   t.DoneAt,
			// Organizer:     &t.CreatedBy, // Disabled until we figure out how this works
			Categories:  categories,
			Start:       t.StartDate,
			End:         t.EndDate,
			Created:     t.Created,
			Updated:     t.Updated,
			DueDate:     t.DueDate,
			Duration:    duration,
			RepeatAfter: t.RepeatAfter,
			RepeatMode:  t.RepeatMode,
			Alarms:      alarms,
			Relations:   relations,
		})
	}

	caldavConfig := &Config{
		Name:   project.Title,
		ProdID: "Vikunja Todo App",
	}

	return ParseTodos(caldavConfig, caldavtodos)
}

// ParsedVTODOProperties reports which task fields the VTODO spoke to, so a partial
// update overlays only those. True only when present *and* understood; an empty value
// is an explicit clear. Labels, reminders and relations signal presence out of band.
type ParsedVTODOProperties struct {
	Title       bool
	Description bool
	Done        bool
	DueDate     bool
	StartDate   bool
	EndDate     bool
}

// ParseTaskFromVTODO parses a VTODO into a Task. Partial updates must overlay only
// the fields reported by the returned properties, or everything else gets wiped.
//
//nolint:gocyclo
func ParseTaskFromVTODO(content string) (vTask *models.Task, props ParsedVTODOProperties, err error) {
	parsed, err := ics.ParseCalendar(strings.NewReader(content))
	if err != nil {
		return nil, props, err
	}
	if len(parsed.Components) == 0 {
		return nil, props, errors.New("VTODO element does seem not contain any components")
	}
	var vTodo *ics.VTodo
	for _, comp := range parsed.Components {
		if todo, ok := comp.(*ics.VTodo); ok {
			vTodo = todo
			break
		}
	}
	if vTodo == nil {
		return nil, props, errors.New("VTODO element not found")
	}
	// We put the vTodo details in a map to be able to handle them more easily
	task := make(map[string]ics.IANAProperty)

	var relations []ics.IANAProperty
	for _, c := range vTodo.UnknownPropertiesIANAProperties() {
		task[c.IANAToken] = c
		if strings.HasPrefix(c.IANAToken, "RELATED-TO") {
			relations = append(relations, c)
		}
	}

	// Get UID and SUMMARY (log warning if missing, but don't fail for backwards compatibility)
	uid, hasUID := task["UID"]
	if !hasUID {
		log.Warningf("[CALDAV] VTODO missing UID field")
	}

	summary, hasSummary := task["SUMMARY"]
	if !hasSummary {
		log.Warningf("[CALDAV] VTODO missing SUMMARY field")
	}

	description := ""
	descProp, hasDescription := task["DESCRIPTION"]
	if hasDescription {
		description = strings.ReplaceAll(descProp.Value, "\\,", ",")
		description = strings.ReplaceAll(description, "\\n", "\n")
	}

	var labels []*models.Label
	if val, ok := task["CATEGORIES"]; ok {
		if val.Value == "" {
			// Empty clears labels, absent leaves them untouched.
			labels = []*models.Label{}
		} else {
			categories := strings.Split(val.Value, ",")
			labels = make([]*models.Label, 0, len(categories))
			for _, category := range categories {
				labels = append(labels, &models.Label{
					Title: category,
				})
			}
		}
	}

	// Safely extract values
	var uidValue, titleValue string
	if hasUID {
		uidValue = uid.Value
	}
	if hasSummary {
		titleValue = summary.Value
	}

	dtStartProp, hasDTStart := task["DTSTART"]
	startDate := caldavTimeToTimestamp(dtStartProp)

	dueProp, hasDue := task["DUE"]
	dueDate := caldavTimeToTimestamp(dueProp)

	// time.ParseDuration can't read RFC 5545 durations like PT120H0M0S, hence ParseISO8601Duration.
	dtEndProp, hasDTEnd := task["DTEND"]
	_, hasDuration := task["DURATION"]
	var endDate time.Time
	var hasEndDate bool
	switch {
	case hasDTEnd:
		endDate = caldavTimeToTimestamp(dtEndProp)
		hasEndDate = parsedDateProperty(dtEndProp, hasDTEnd, endDate)
	case hasDuration && hasDTStart && !startDate.IsZero():
		if d := utils.ParseISO8601Duration(task["DURATION"].Value); d > 0 {
			endDate = startDate.Add(d)
			hasEndDate = true
		}
	}

	vTask = &models.Task{
		UID:         uidValue,
		Title:       titleValue,
		Description: description,
		Labels:      labels,
		DueDate:     dueDate,
		Updated:     caldavTimeToTimestamp(task["DTSTAMP"]),
		StartDate:   startDate,
		EndDate:     endDate,
		DoneAt:      caldavTimeToTimestamp(task["COMPLETED"]),
	}

	for _, c := range relations {
		var relTypeStr string
		if _, ok := c.ICalParameters["RELTYPE"]; ok {
			if len(c.ICalParameters["RELTYPE"]) != 1 {
				continue
			}

			relTypeStr = c.ICalParameters["RELTYPE"][0]
		}

		var relationKind models.RelationKind
		switch relTypeStr {
		case "PARENT":
			relationKind = models.RelationKindParenttask
		case "CHILD":
			relationKind = models.RelationKindSubtask
		default:
			relationKind = models.RelationKindParenttask
		}

		if vTask.RelatedTasks == nil {
			vTask.RelatedTasks = make(map[models.RelationKind][]*models.Task)
		}

		vTask.RelatedTasks[relationKind] = append(vTask.RelatedTasks[relationKind], &models.Task{
			UID: c.Value,
		})
	}

	_, hasStatus := task["STATUS"]
	_, hasCompleted := task["COMPLETED"]

	// RFC 5545: COMPLETED alone implies done; explicit STATUS always wins.
	if hasCompleted {
		vTask.Done = true
	}
	if status, ok := task["STATUS"]; ok {
		vTask.Done = status.Value == "COMPLETED"
	}

	for _, vAlarm := range vTodo.SubComponents() {
		if vAlarm, ok := vAlarm.(*ics.VAlarm); ok {
			vTask = parseVAlarm(vAlarm, vTask)
		}
	}

	props = ParsedVTODOProperties{
		Title:       hasSummary,
		Description: hasDescription,
		Done:        hasStatus || hasCompleted,
		DueDate:     parsedDateProperty(dueProp, hasDue, dueDate),
		StartDate:   parsedDateProperty(dtStartProp, hasDTStart, startDate),
		EndDate:     hasEndDate,
	}

	return vTask, props, nil
}

// parsedDateProperty tells an explicit clear (empty value) apart from a value
// caldavTimeToTimestamp couldn't read - both yield the zero time.
func parsedDateProperty(prop ics.IANAProperty, present bool, parsed time.Time) bool {
	return present && (prop.Value == "" || !parsed.IsZero())
}

func parseVAlarm(vAlarm *ics.VAlarm, vTask *models.Task) *models.Task {
	for _, property := range vAlarm.UnknownPropertiesIANAProperties() {
		if property.IANAToken != "TRIGGER" {
			continue
		}

		if contains(property.ICalParameters["VALUE"], "DATE-TIME") {
			// Example: TRIGGER;VALUE=DATE-TIME:20181201T011210Z
			vTask.Reminders = append(vTask.Reminders, &models.TaskReminder{
				Reminder: caldavTimeToTimestamp(property),
			})
			continue
		}

		duration := utils.ParseISO8601Duration(property.Value)

		if contains(property.ICalParameters["RELATED"], "END") {
			// Example: TRIGGER;RELATED=END:-P2D
			// We emit due- and end-relative reminders both as RELATED=END, so prefer due_date to keep ours round-tripping.
			relativeTo := models.ReminderRelationEndDate
			if !vTask.DueDate.IsZero() || vTask.EndDate.IsZero() {
				relativeTo = models.ReminderRelationDueDate
			}
			vTask.Reminders = append(vTask.Reminders, &models.TaskReminder{
				RelativePeriod: int64(duration.Seconds()),
				RelativeTo:     relativeTo})
			continue
		}

		// Example: TRIGGER;RELATED=START:-P2D
		// Example: TRIGGER:-PT60M
		vTask.Reminders = append(vTask.Reminders, &models.TaskReminder{
			RelativePeriod: int64(duration.Seconds()),
			RelativeTo:     models.ReminderRelationStartDate})
	}
	return vTask
}

func contains(array []string, str string) bool {
	for _, value := range array {
		if value == str {
			return true
		}
	}
	return false
}

// https://tools.ietf.org/html/rfc5545#section-3.3.5
func caldavTimeToTimestamp(ianaProperty ics.IANAProperty) time.Time {
	tstring := ianaProperty.Value
	if tstring == "" {
		return time.Time{}
	}

	format := DateFormat

	if strings.HasSuffix(tstring, "Z") {
		format = `20060102T150405Z`
	}

	if len(tstring) == 8 {
		format = `20060102`
	}

	var t time.Time
	var err error
	tzParameter := ianaProperty.ICalParameters["TZID"]
	if len(tzParameter) > 0 {
		loc, locErr := time.LoadLocation(tzParameter[0])
		if locErr != nil {
			log.Warningf("Error while parsing caldav timezone %s: %s", tzParameter[0], locErr)
		} else {
			t, err = time.ParseInLocation(format, tstring, loc)
		}
	} else {
		t, err = time.ParseInLocation(format, tstring, config.GetTimeZone())
	}

	if err != nil {
		log.Warningf("Error while parsing caldav time %s to TimeStamp: %s", tstring, err)
		return time.Time{}
	}

	return t.In(config.GetTimeZone())
}
