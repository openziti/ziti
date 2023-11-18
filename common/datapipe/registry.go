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

package datapipe

import (
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"sync"
	"sync/atomic"
)

type Pipe interface {
	Id() uint32
	WriteToServer(data []byte) error
	WriteToClient(data []byte) error
	CloseWithErr(err error)
}

func NewRegistry(config *Config) *Registry {
	return &Registry{
		config: config,
	}
}

type Registry struct {
	lock        sync.Mutex
	nextId      atomic.Uint32
	connections concurrenz.CopyOnWriteMap[uint32, Pipe]
	config      *Config
}

func (self *Registry) GetConfig() *Config {
	return self.config
}

func (self *Registry) GetNextId() (uint32, error) {
	self.lock.Lock()
	defer self.lock.Unlock()

	limit := 0
	for {
		nextId := self.nextId.Add(1)
		if val := self.connections.Get(nextId); val == nil {
			return nextId, nil
		}
		if limit++; limit >= 1000 {
			return 0, errors.New("pipe pool in bad state, bailing out after 1000 attempts to get next id")
		}
	}
}

func (self *Registry) Register(pipe Pipe) error {
	self.lock.Lock()
	defer self.lock.Unlock()

	if self.connections.Get(pipe.Id()) != nil {
		pfxlog.Logger().Errorf("pipe already registered (id=%d)", pipe.Id())
		return fmt.Errorf("pipe already registered")
	}

	self.connections.Put(pipe.Id(), pipe)
	return nil
}

func (self *Registry) Unregister(id uint32) {
	self.connections.Delete(id)
}

func (self *Registry) Get(id uint32) Pipe {
	return self.connections.Get(id)
}
