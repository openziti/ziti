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

type Service struct {
	BaseDbEntity
	Name            *string
	DnsHostname     *string
	DnsPort         *uint16
	Clusters        []*Cluster `gorm:"many2many:service_clusters;"`
	AppWans         []*AppWan  `gorm:"many2many:app_wan_services;"`
	EndpointAddress *string
	EgressRouter    *string
	HostIds         []*Identity `gorm:"many2many:service_hosts"`
}

func (e *Service) deleteNetworkSessions(tx *gorm.DB) error {
	sql, args, err := squirrel.Eq{"service_id": e.ID}.ToSql()

	if err != nil {
		return err
	}

	ns := make([]*NetworkSession, 0)

	res := tx.Where(sql, args).Find(&ns)

	if res.Error != nil {
		return res.Error
	}

	for _, n := range ns {
		tx.Delete(n)
	}

	return nil
}

func (e *Service) BeforeDelete(tx *gorm.DB) error {

	if err := e.ClearAssociations(e, tx, "AppWans", "Clusters", "HostIds"); err != nil {
		return err
	}

	if err := e.deleteNetworkSessions(tx); err != nil {
		return err
	}

	return nil
}
