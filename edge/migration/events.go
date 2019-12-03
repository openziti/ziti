/*
	Copyright 2019 Netfoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package migration

import "github.com/kataras/go-events"

const (
	EventCreate events.EventName = "CREATE"
	EventDelete events.EventName = "DELETE"
	EventUpdate events.EventName = "UPDATE"
	EventPatch  events.EventName = "PATCH"
)

type CrudEventDetails struct {
	Entities       []BaseDbModel
	QueryResult    *QueryResult
	FieldsAffected []string
}

func NewCrudEventDetails(qr *QueryResult, entities ...BaseDbModel) *CrudEventDetails {
	return &CrudEventDetails{
		Entities:    entities,
		QueryResult: qr,
	}
}
