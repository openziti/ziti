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

type AppWan struct {
	BaseDbEntity
	Name       *string
	Identities []*Identity `gorm:"many2many:app_wan_identities"`
	Services   []*Service  `gorm:"many2many:app_wan_services"`
}

func (e *AppWan) BeforeDelete(tx *gorm.DB) error {

	res := tx.Model(e).Association("Identities").Clear()

	if res.Error != nil {
		return res.Error
	}

	res = tx.Model(e).Association("Services").Clear()

	return res.Error
}
