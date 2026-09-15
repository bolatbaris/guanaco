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

package apiv2

import (
	"context"
	"net/http"

	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/web/handler"

	"github.com/danielgtaylor/huma/v2"
)

// {entity} stays a string so Can{Create,Delete} can derive the numeric entity
// type from it. The enum tag makes Huma reject anything other than project.
type subscriptionPathParams struct {
	Entity   string `path:"entity" enum:"project" doc:"The project to (un)subscribe from."`
	EntityID int64  `path:"entityID" doc:"The numeric id of the project to (un)subscribe from."`
}

func RegisterSubscriptionRoutes(api huma.API) {
	tags := []string{"subscriptions"}

	Register(api, huma.Operation{
		OperationID: "subscriptions-create",
		Summary:     "Subscribe to a project",
		Description: "Subscribes the authenticated user to a project so they receive its notifications. The user needs read access to the project. Fails if the user is already subscribed, directly or through a parent project. Subscribing again after an opt-out lifts it.",
		Method:      http.MethodPost,
		Path:        "/subscriptions/{entity}/{entityID}",
		Tags:        tags,
	}, subscriptionsCreate)

	Register(api, huma.Operation{
		OperationID: "subscriptions-delete",
		Summary:     "Unsubscribe from a project",
		Description: "Stops notifications about a project for the authenticated user. If the subscription was inherited from a parent project, the opt-out is recorded for this project instead, leaving the parent subscription in place. Only affects the caller's subscription, not other users'.",
		Method:      http.MethodDelete,
		Path:        "/subscriptions/{entity}/{entityID}",
		Tags:        tags,
	}, subscriptionsDelete)
}

func init() { AddRouteRegistrar(RegisterSubscriptionRoutes) }

func subscriptionsCreate(ctx context.Context, in *subscriptionPathParams) (*singleBody[models.Subscription], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	sb := &models.Subscription{Entity: in.Entity, EntityID: in.EntityID}
	if err := handler.DoCreate(ctx, sb, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.Subscription]{Body: sb}, nil
}

func subscriptionsDelete(ctx context.Context, in *subscriptionPathParams) (*emptyBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	sb := &models.Subscription{Entity: in.Entity, EntityID: in.EntityID}
	if err := handler.DoDelete(ctx, sb, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &emptyBody{}, nil
}
