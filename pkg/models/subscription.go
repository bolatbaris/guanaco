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
	"encoding/json"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"github.com/danielgtaylor/huma/v2"
	"xorm.io/xorm"
)

// SubscriptionEntityType represents all entities which can be subscribed to
type SubscriptionEntityType int

const (
	SubscriptionEntityUnknown   SubscriptionEntityType = 0
	SubscriptionEntityNamespace SubscriptionEntityType = 1 // Kept for compatibility with stored ids.
	SubscriptionEntityProject   SubscriptionEntityType = 2
)

func (st *SubscriptionEntityType) UnmarshalJSON(bytes []byte) error {
	var value string
	err := json.Unmarshal(bytes, &value)
	if err != nil {
		return err
	}

	switch value {
	case "project":
		*st = SubscriptionEntityProject
	default:
		return &ErrUnknownSubscriptionEntityType{EntityType: *st}
	}

	return nil
}

func (st SubscriptionEntityType) MarshalJSON() ([]byte, error) {
	switch st {
	case SubscriptionEntityProject:
		return []byte(`"project"`), nil
	case SubscriptionEntityUnknown, SubscriptionEntityNamespace:
	}

	return []byte(`null`), nil
}

// Schema lets Huma (/api/v2) reflect this type as a string enum; see the note
// on ProjectViewKind.Schema for why this is needed.
func (*SubscriptionEntityType) Schema(_ huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type: "string",
		Enum: []any{"project"},
	}
}

func getEntityTypeFromString(entityType string) SubscriptionEntityType {
	if entityType == entityProject {
		return SubscriptionEntityProject
	}

	return SubscriptionEntityUnknown
}

func (st SubscriptionEntityType) validate() error {
	if st == SubscriptionEntityProject {
		return nil
	}

	return &ErrUnknownSubscriptionEntityType{EntityType: st}
}

const (
	entityProject = `project`
)

// Subscription represents a subscription for an entity
type Subscription struct {
	// The numeric ID of the subscription
	ID int64 `xorm:"autoincr not null unique pk" json:"id" readOnly:"true" doc:"The numeric id of the subscription."`

	EntityType SubscriptionEntityType `xorm:"index not null unique(entity_user)" json:"entity" readOnly:"true" doc:"The kind of entity this subscription is for: project, derived server-side from the request path."`
	Entity     string                 `xorm:"-" json:"-" param:"entity"`
	// The id of the entity to subscribe to.
	EntityID int64 `xorm:"bigint index not null unique(entity_user)" json:"entity_id" param:"entityID" readOnly:"true" doc:"The numeric id of the subscribed entity; taken from the request path."`

	// The user who made this subscription
	UserID int64 `xorm:"bigint index not null unique(entity_user)" json:"-"`

	// Muted turns the row into an opt-out: it outranks any inherited subscription and is dropped while resolving.
	Muted bool `xorm:"not null default false" json:"-" xml:"-"`

	// A timestamp when this subscription was created. You cannot change this value.
	Created time.Time `xorm:"created not null" json:"created" readOnly:"true" doc:"A timestamp when this subscription was created. You cannot change this value."`

	web.CRUDable    `xorm:"-" json:"-"`
	web.Permissions `xorm:"-" json:"-"`
}

type SubscriptionWithUser struct {
	Subscription `xorm:"extends"`
	User         *user.User `xorm:"extends" json:"user"`
}

type subscriptionResolved struct {
	OriginalEntityID     int64
	SubscriptionID       int64
	SubscriptionWithUser `xorm:"extends"`
}

// TableName gives us a better table name for the subscriptions table
func (sb *Subscription) TableName() string {
	return "subscriptions"
}

// Create subscribes the current user to a project.
// @Summary Subscribes the current user to a project.
// @Description Subscribes the current user to a project.
// @tags subscriptions
// @Accept json
// @Produce json
// @Security JWTKeyAuth
// @Param entity path string true "The project the user subscribes to."
// @Param entityID path string true "The numeric id of the project to subscribe to."
// @Success 201 {object} models.Subscription "The subscription"
// @Failure 403 {object} web.HTTPError "The user does not have access to subscribe to this project."
// @Failure 412 {object} web.HTTPError "The subscription already exists."
// @Failure 412 {object} web.HTTPError "The subscription entity is invalid."
// @Failure 500 {object} models.Message "Internal error"
// @Router /subscriptions/{entity}/{entityID} [put]
func (sb *Subscription) Create(s *xorm.Session, auth web.Auth) (err error) {
	// Permissions method already does the validation of the entity type, so we don't need to do that here

	sb.ID = 0
	sb.UserID = auth.GetID()
	sb.Muted = false

	own, err := getOwnSubscription(s, sb.EntityType, sb.EntityID, sb.UserID)
	if err != nil {
		return err
	}
	if own != nil {
		if !own.Muted {
			return &ErrSubscriptionAlreadyExists{
				EntityID:   sb.EntityID,
				EntityType: sb.EntityType,
				UserID:     sb.UserID,
			}
		}

		// Subscribing again lifts a previous opt-out.
		sb.ID = own.ID
		sb.Created = time.Now().UTC()
		_, err = s.
			Where("entity_id = ? AND entity_type = ? AND user_id = ?", sb.EntityID, sb.EntityType, sb.UserID).
			Cols("muted", "created").
			Update(&Subscription{Muted: false, Created: sb.Created})
		return err
	}

	// Without an own row, only a parent entity can still make the user subscribed.
	sub, err := GetSubscriptionForUser(s, sb.EntityType, sb.EntityID, auth)
	if err != nil {
		return err
	}
	if sub != nil {
		return &ErrSubscriptionAlreadyExists{
			EntityID:   sub.EntityID,
			EntityType: sub.EntityType,
			UserID:     sub.UserID,
		}
	}

	_, err = s.Insert(sb)
	return
}

// Delete unsubscribes the current user from a project.
// @Summary Unsubscribe the current user from a project.
// @Description Unsubscribes the current user from a project. If the subscription is inherited from a parent project, an opt-out is stored for this project instead.
// @tags subscriptions
// @Accept json
// @Produce json
// @Security JWTKeyAuth
// @Param entity path string true "The project the user subscribed to."
// @Param entityID path string true "The numeric id of the subscribed project."
// @Success 200 {object} models.Subscription "The subscription"
// @Failure 403 {object} web.HTTPError "The user does not have access to unsubscribe from this project."
// @Failure 404 {object} web.HTTPError "The subscription does not exist."
// @Failure 500 {object} models.Message "Internal error"
// @Router /subscriptions/{entity}/{entityID} [delete]
func (sb *Subscription) Delete(s *xorm.Session, auth web.Auth) (err error) {
	if err := sb.EntityType.validate(); err != nil {
		return err
	}

	sb.UserID = auth.GetID()

	_, err = s.
		Where("entity_id = ? AND entity_type = ? AND user_id = ?", sb.EntityID, sb.EntityType, sb.UserID).
		Delete(&Subscription{})
	if err != nil {
		return err
	}

	// Removing the row can uncover a parent entity's subscription, which only an explicit opt-out overrides.
	inherited, err := GetSubscriptionForUser(s, sb.EntityType, sb.EntityID, auth)
	if err != nil || inherited == nil {
		return err
	}

	// CanDelete lets a user who lost access clean up their own row, so gate only the opt-out insert.
	canRead, err := sb.canReadEntity(s, auth)
	if err != nil || !canRead {
		return err
	}

	sb.ID = 0
	sb.Muted = true
	_, err = s.Insert(sb)
	return err
}

// getOwnSubscription returns the row for exactly this entity, ignoring inherited subscriptions.
func getOwnSubscription(s *xorm.Session, entityType SubscriptionEntityType, entityID, userID int64) (subscription *Subscription, err error) {
	subscription = &Subscription{}
	exists, err := s.
		Where("entity_id = ? AND entity_type = ? AND user_id = ?", entityID, entityType, userID).
		Get(subscription)
	if err != nil || !exists {
		return nil, err
	}

	return subscription, nil
}

func GetSubscriptionForUser(s *xorm.Session, entityType SubscriptionEntityType, entityID int64, a web.Auth) (subscription *SubscriptionWithUser, err error) {
	u, is := a.(*user.User)
	if !is || u == nil {
		return
	}

	subs, err := GetSubscriptionsForEntitiesAndUser(s, entityType, []int64{entityID}, u)
	if err != nil || len(subs) == 0 || len(subs[entityID]) == 0 {
		return nil, err
	}

	return subs[entityID][0], nil
}

// GetSubscriptionsForEntities returns a list of subscriptions to for an entity ID
func GetSubscriptionsForEntities(s *xorm.Session, entityType SubscriptionEntityType, entityIDs []int64) (subscriptions map[int64][]*SubscriptionWithUser, err error) {
	return getSubscriptionsForEntitiesAndUser(s, entityType, entityIDs, nil, false)
}

func GetSubscriptionsForEntitiesAndUser(s *xorm.Session, entityType SubscriptionEntityType, entityIDs []int64, u *user.User) (subscriptions map[int64][]*SubscriptionWithUser, err error) {
	return getSubscriptionsForEntitiesAndUser(s, entityType, entityIDs, u, true)
}

func GetSubscriptionsForEntity(s *xorm.Session, entityType SubscriptionEntityType, entityID int64) (subscriptions []*SubscriptionWithUser, err error) {
	subs, err := GetSubscriptionsForEntities(s, entityType, []int64{entityID})
	if err != nil || len(subs[entityID]) == 0 {
		return
	}

	return subs[entityID], nil
}

// This function returns a matching subscription for an entity and user.
// It resolves direct and inherited project subscriptions.
// It will return a map where the key is the entity id and the value is a slice with all subscriptions for that entity.
func getSubscriptionsForEntitiesAndUser(s *xorm.Session, entityType SubscriptionEntityType, entityIDs []int64, u *user.User, userOnly bool) (subscriptions map[int64][]*SubscriptionWithUser, err error) {
	if err := entityType.validate(); err != nil {
		return nil, err
	}

	rawSubscriptions := []*subscriptionResolved{}
	idList := strings.TrimSuffix(strings.Repeat("?, ", len(entityIDs)), ", ")
	idArgs := make([]any, 0, len(entityIDs))
	for _, id := range entityIDs {
		idArgs = append(idArgs, id)
	}
	var args []any
	// arguments must follow the order of the placeholders in the query text
	add := func(vals ...any) { args = append(args, vals...) }

	var sUserCond string
	var sUserArgs []any
	if userOnly {
		if u == nil {
			return nil, &ErrMustProvideUser{}
		}
		sUserCond = " AND s.user_id = ?"
		sUserArgs = []any{u.ID}
	}

	switch entityType {
	case SubscriptionEntityProject:
		add(idArgs...)
		add(SubscriptionEntityProject)
		add(sUserArgs...)
		add(idArgs...)
		err = s.SQL(`
WITH RECURSIVE project_hierarchy AS (
    -- Base case: Start with the specified projects
    SELECT
        id,
        parent_project_id,
        0 AS level,
        id AS original_project_id
    FROM projects
    WHERE id IN (`+idList+`)

    UNION ALL

    -- Recursive case: Get parent projects
    SELECT
        p.id,
        p.parent_project_id,
        ph.level + 1,
        ph.original_project_id
    FROM projects p
             INNER JOIN project_hierarchy ph ON p.id = ph.parent_project_id
),

subscription_hierarchy AS (
    -- Check for project subscriptions (including parent projects)
    SELECT
        s.id,
        s.entity_type,
        s.entity_id,
        s.created,
        s.user_id,
        s.muted,
        CASE
            WHEN s.entity_id = ph.original_project_id THEN 1  -- Direct project match
            ELSE ph.level + 1  -- Parent projects
            END AS priority,
        ph.original_project_id
    FROM subscriptions s
             INNER JOIN project_hierarchy ph ON s.entity_id = ph.id
    WHERE s.entity_type = ?`+sUserCond+`
)

SELECT
    p.id AS original_entity_id,
    sh.id AS subscription_id,
    sh.entity_type,
    sh.entity_id,
    sh.created,
    sh.user_id,
    sh.muted,
    CASE
        WHEN sh.priority = 1 THEN 'Direct Project'
        ELSE 'Parent Project'
        END 
	AS subscription_level,
    users.*
FROM projects p
         LEFT JOIN (
    SELECT *,
           ROW_NUMBER() OVER (PARTITION BY original_project_id, user_id ORDER BY priority) AS rn
    FROM subscription_hierarchy
) sh ON p.id = sh.original_project_id AND sh.rn = 1
    LEFT JOIN users ON sh.user_id = users.id
WHERE p.id IN (`+idList+`)
		ORDER BY p.id, sh.user_id`, args...).
			Find(&rawSubscriptions)
	case SubscriptionEntityUnknown, SubscriptionEntityNamespace:
		return nil, &ErrUnknownSubscriptionEntityType{EntityType: entityType}
	}
	if err != nil {
		return nil, err
	}

	subscriptions = make(map[int64][]*SubscriptionWithUser)
	for _, sub := range rawSubscriptions {

		if sub.EntityID == 0 {
			continue
		}

		// Already outranked the parent's subscription, so dropping it unsubscribes the user.
		if sub.Muted {
			continue
		}

		_, has := subscriptions[sub.OriginalEntityID]
		if !has {
			subscriptions[sub.OriginalEntityID] = []*SubscriptionWithUser{}
		}

		sub.ID = sub.SubscriptionID
		if sub.User != nil {
			sub.User.ID = sub.UserID
		}

		subscriptions[sub.OriginalEntityID] = append(subscriptions[sub.OriginalEntityID], &sub.SubscriptionWithUser)
	}

	if userOnly {
		return subscriptions, nil
	}

	return filterSubscriptionsByReadPermission(s, subscriptions)
}

// Subscription rows outlive access, so subscribers who lost read access to the entity's project are
// filtered out here rather than deleted: a subscription is user intent and resumes once access returns.
func filterSubscriptionsByReadPermission(s *xorm.Session, subscriptions map[int64][]*SubscriptionWithUser) (map[int64][]*SubscriptionWithUser, error) {
	if len(subscriptions) == 0 {
		return subscriptions, nil
	}

	entityIDs := make([]int64, 0, len(subscriptions))
	for entityID := range subscriptions {
		entityIDs = append(entityIDs, entityID)
	}

	projectIDForEntity := make(map[int64]int64, len(subscriptions))
	for _, entityID := range entityIDs {
		projectIDForEntity[entityID] = entityID
	}

	return filterSubscriptionsByProjectPermission(s, subscriptions, projectIDForEntity)
}

// filterSubscriptionsByProjectPermission drops subscribers who can no longer read
// the project each entity belongs to.
func filterSubscriptionsByProjectPermission(s *xorm.Session, subscriptions map[int64][]*SubscriptionWithUser, projectIDForEntity map[int64]int64) (map[int64][]*SubscriptionWithUser, error) {
	subscribers := make(map[int64]*user.User)
	projectIDs := make([]int64, 0, len(projectIDForEntity))
	seenProjectID := make(map[int64]bool, len(projectIDForEntity))
	for entityID, subs := range subscriptions {
		projectID, has := projectIDForEntity[entityID]
		if !has {
			continue
		}

		for _, sub := range subs {
			if sub.User == nil {
				continue
			}

			if _, has := subscribers[sub.User.ID]; !has {
				subscribers[sub.User.ID] = sub.User
			}
			if !seenProjectID[projectID] {
				seenProjectID[projectID] = true
				projectIDs = append(projectIDs, projectID)
			}
		}
	}

	readable := make(map[int64]map[int64]*projectReadPermission, len(subscribers))
	for userID, u := range subscribers {
		permissions, err := checkReadPermissionsForProjects(s, u, projectIDs)
		if err != nil {
			return nil, err
		}
		readable[userID] = permissions
	}

	filtered := make(map[int64][]*SubscriptionWithUser, len(subscriptions))
	for entityID, subs := range subscriptions {
		projectID, has := projectIDForEntity[entityID]
		if !has {
			continue
		}

		for _, sub := range subs {
			if sub.User == nil {
				continue
			}

			permission, has := readable[sub.User.ID][projectID]
			if !has || !permission.canRead {
				continue
			}

			filtered[entityID] = append(filtered[entityID], sub)
		}
	}

	return filtered, nil
}
