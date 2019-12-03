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

type Session struct {
	BaseDbEntity
	IdentityID *string
	Token      *string
	Identity   *Identity
}

func (e *Session) BeforeDelete(tx *gorm.DB) error {
	store := getDefaultNetworkSessionGormStore()

	sql, args, err := squirrel.Eq{"session_id": e.ID}.ToSql()

	if err != nil {
		return nil
	}
	ns := make([]*NetworkSession, 0)

	res := tx.Where(sql, args).Find(&ns)

	if res.Error != nil {
		return res.Error
	}

	for _, n := range ns {
		if err := store.DeleteWithTx(n, tx); err != nil {
			return err
		}
	}

	return res.Error
}
