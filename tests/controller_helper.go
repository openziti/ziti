/*
	Copyright NetFoundry Inc.

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

package tests

import (
	"time"

	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/server"
)

// ControllerHelper wraps a server.Controller with test-oriented helper methods.
// All methods return values and errors; none accept *testing.T.
// Because it embeds *server.Controller, all controller methods are available directly.
type ControllerHelper struct {
	*server.Controller
}

// ReadRevocation returns the revocation with the given id from the controller DB,
// or (nil, nil) if not found.
func (h *ControllerHelper) ReadRevocation(id string) (*model.Revocation, error) {
	rev, err := h.AppEnv.GetManagers().Revocation.Read(id)
	if boltz.IsErrNotFoundErr(err) {
		return nil, nil
	}
	return rev, err
}

// CreateRevocation writes a revocation entry with the given id and ExpiresAt directly
// to the controller DB. Useful for planting entries (e.g. with a past ExpiresAt) in tests.
func (h *ControllerHelper) CreateRevocation(id string, expiresAt time.Time) error {
	ctx := change.New().SetSourceType("test").SetChangeAuthorType(change.AuthorTypeController)
	return h.AppEnv.GetManagers().Revocation.Create(&model.Revocation{
		BaseEntity: models.BaseEntity{Id: id},
		ExpiresAt:  expiresAt,
	}, ctx)
}

// DeleteExpiredRevocations calls DeleteExpired on the revocation manager and returns
// the total number of entries removed.
func (h *ControllerHelper) DeleteExpiredRevocations() (int, error) {
	ctx := change.New().SetSourceType("test").SetChangeAuthorType(change.AuthorTypeController)
	return h.AppEnv.GetManagers().Revocation.DeleteExpired(ctx)
}

// FlushRevocationBatcher synchronously drains any pending batched revocations
// so that tests can verify they were persisted.
func (h *ControllerHelper) FlushRevocationBatcher() {
	if f := h.AppEnv.GetManagers().RevocationBatchFlusher; f != nil {
		f()
	}
}
