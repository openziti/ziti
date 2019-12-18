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
	"gopkg.in/Masterminds/squirrel.v1"
)

type IdentityDeleteHandler func(tx *gorm.DB, i *Identity) error

type IdentityHandlers struct {
	deletes []IdentityDeleteHandler
}

var identityHandlersInstance = &IdentityHandlers{}

func (ih *IdentityHandlers) AddDeleteHandler(handler IdentityDeleteHandler) {
	ih.deletes = append(ih.deletes, handler)
}

func (ih *IdentityHandlers) Delete(tx *gorm.DB, i *Identity) error {
	for _, h := range ih.deletes {
		err := h(tx, i)

		if err != nil {
			return err
		}
	}

	return nil
}

type Identity struct {
	BaseDbEntity
	Name           *string
	TypeID         *string `gorm:"column:identity_type_id"`
	Type           *IdentityType
	IsDefaultAdmin *bool
	IsAdmin        *bool
	Enrollments    []*Enrollment
	Authenticators []*Authenticator
	Permissions    []string  `gorm:"-"`
	AppWans        []*AppWan `gorm:"many2many:app_wan_identities"`
}

func (e *Identity) deleteSessions(tx *gorm.DB) error {
	sql, args, err := squirrel.Eq{"identity_id": e.ID}.ToSql()

	if err != nil {
		return err
	}

	ss := make([]*Session, 0)

	res := tx.Where(sql, args).Find(&ss)

	if res.Error != nil {
		return res.Error
	}

	for _, s := range ss {
		tx.Delete(s)
	}

	return nil
}

func (e *Identity) deleteEnrollments(tx *gorm.DB) error {
	sql, args, err := squirrel.Eq{"identity_id": e.ID}.ToSql()

	if err != nil {
		return err
	}

	ens := make([]*Enrollment, 0)

	res := tx.Where(sql, args).Find(&ens)

	if res.Error != nil {
		return res.Error
	}

	for _, en := range ens {
		if res := tx.Delete(en); res.Error != nil {
			return res.Error
		}
	}

	return nil
}

func (e *Identity) deleteAuthentciators(tx *gorm.DB) error {
	sql, args, err := squirrel.Eq{"identity_id": e.ID}.ToSql()

	if err != nil {
		return err
	}

	ens := make([]*Authenticator, 0)

	res := tx.Where(sql, args).Find(&ens)

	if res.Error != nil {
		return res.Error
	}

	for _, en := range ens {
		if res := tx.Delete(en); res.Error != nil {
			return res.Error
		}
	}

	return nil
}

func (e *Identity) deleteAppWanAssociations(tx *gorm.DB) error {

	res := tx.Model(e).Association("AppWans").Clear()

	if res.Error != nil {
		return res.Error
	}

	return nil
}

func (e *Identity) BeforeDelete(tx *gorm.DB) error {
	err := e.deleteSessions(tx)

	if err != nil {
		return err
	}

	err = e.deleteEnrollments(tx)

	if err != nil {
		return err
	}

	err = e.deleteAuthentciators(tx)

	if err != nil {
		return err
	}

	err = e.deleteAppWanAssociations(tx)

	if err != nil {
		return err
	}

	err = identityHandlersInstance.Delete(tx, e)

	if err != nil {
		return err
	}
	return nil
}

func (e *Identity) AfterCreate(tx *gorm.DB) error {
	for _, en := range e.Enrollments {
		err := en.Enroll(e, tx)

		if err != nil {
			return err
		}
	}

	return nil
}
