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

type NetworkSession struct {
	BaseDbEntity
	Token     *string
	SessionID *string
	Session   *Session
	ServiceID *string
	Hosting   *bool
	Certs     []*NetworkSessionCert
}

func (e *NetworkSession) deleteCerts(tx *gorm.DB) error {
	sql, args, err := squirrel.Eq{"network_session_id": e.ID}.ToSql()

	if err != nil {
		return err
	}

	tx.Delete(&NetworkSessionCert{}, sql, args)

	return nil
}

func (e *NetworkSession) BeforeDelete(tx *gorm.DB) error {
	if err := e.deleteCerts(tx); err != nil {
		return err
	}
	return nil
}
