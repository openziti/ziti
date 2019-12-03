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
	"time"
)

type Enrollment struct {
	BaseDbEntity
	IdentityID *string
	Token      *string
	Method     *string
	Config     interface{}                          `gorm:"-"`
	Enroll     func(i *Identity, tx *gorm.DB) error `gorm:"-"`
	ExpiresAt  *time.Time
}

func (e *Enrollment) BeforeDelete(tx *gorm.DB) error {
	return enrollmentHandlersInstance.Delete(tx, e)
}

type EnrollmentDeleteHandler func(tx *gorm.DB, i *Enrollment) error

type EnrollmentHandlers struct {
	deletes []EnrollmentDeleteHandler
}

var enrollmentHandlersInstance = &EnrollmentHandlers{}

func (ih *EnrollmentHandlers) AddDeleteHandler(handler EnrollmentDeleteHandler) {
	ih.deletes = append(ih.deletes, handler)
}

func (ih *EnrollmentHandlers) Delete(tx *gorm.DB, i *Enrollment) error {
	for _, h := range ih.deletes {
		err := h(tx, i)

		if err != nil {
			return err
		}
	}

	return nil
}
