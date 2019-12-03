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

type CaDeleteHandler func(tx *gorm.DB, ca *Ca) error

type CaHandlers struct {
	deletes []CaDeleteHandler
}

var caHandlersInstance = &CaHandlers{}

func (cah *CaHandlers) AddDeleteHandler(handler CaDeleteHandler) {
	cah.deletes = append(cah.deletes, handler)
}

func (cah *CaHandlers) Delete(tx *gorm.DB, ca *Ca) error {
	for _, h := range cah.deletes {
		err := h(tx, ca)

		if err != nil {
			return err
		}
	}

	return nil
}

type Ca struct {
	BaseDbEntity
	Name                      *string
	Fingerprint               *string
	CertPem                   *string
	IsVerified                *bool
	VerificationToken         *string
	IsAutoCaEnrollmentEnabled *bool
	IsOttCaEnrollmentEnabled  *bool
	IsAuthEnabled             *bool
}

func (e *Ca) BeforeDelete(tx *gorm.DB) error {
	if err := caHandlersInstance.Delete(tx, e); err != nil {
		return err
	}

	return nil
}
