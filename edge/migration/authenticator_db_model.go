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

import (
	"github.com/jinzhu/gorm"
)

type Authenticator struct {
	BaseDbEntity
	IdentityID *string
	Method     *string
}

func (e *Authenticator) BeforeDelete(tx *gorm.DB) error {
	return authenticatorHandlersInstance.Delete(tx, e)
}

type AuthenticatorDeleteHandler func(tx *gorm.DB, i *Authenticator) error

type AuthenticatorHandlers struct {
	deletes []AuthenticatorDeleteHandler
}

var authenticatorHandlersInstance = &AuthenticatorHandlers{}

func (ih *AuthenticatorHandlers) AddDeleteHandler(handler AuthenticatorDeleteHandler) {
	ih.deletes = append(ih.deletes, handler)
}

func (ih *AuthenticatorHandlers) Delete(tx *gorm.DB, i *Authenticator) error {
	for _, h := range ih.deletes {
		err := h(tx, i)

		if err != nil {
			return err
		}
	}

	return nil
}
