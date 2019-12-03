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

package model

import (
	"github.com/netfoundry/ziti-edge/edge/controller/persistence"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestBoltToModelCopy(t *testing.T) {
	req := require.New(t)
	boltEntity := &persistence.Cluster{
		BaseEdgeEntityImpl: persistence.BaseEdgeEntityImpl{
			Id: uuid.New().String(),
			EdgeEntityFields: persistence.EdgeEntityFields{
				CreatedAt: time.Now(),
				UpdatedAt: time.Now().Add(time.Second),
				Tags:      nil,
			},
		},
		Name: uuid.New().String(),
	}

	modelEntity := &Cluster{}
	err := modelEntity.FillFrom(nil, nil, boltEntity)
	req.NoError(err)

	req.Equal(boltEntity.Id, modelEntity.Id)
	req.Equal(boltEntity.CreatedAt, modelEntity.CreatedAt)
	req.Equal(boltEntity.UpdatedAt, modelEntity.UpdatedAt)
	req.Equal(boltEntity.Tags, modelEntity.Tags)
	req.Equal(boltEntity.Name, modelEntity.Name)
}
