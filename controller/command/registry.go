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

package command

import (
	"encoding/binary"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/pkg/errors"
)

var defaultDecoders = NewDecoders()

func GetDefaultDecoders() Decoders {
	return defaultDecoders
}

type Decoders interface {
	Register(id int32, decoder Decoder)
	RegisterF(id int32, decoder DecoderF)
	Decode(data []byte) (Command, error)
	GetDecoder(id int32) Decoder
	Clear()
}

func NewDecoders() Decoders {
	result := &decodersImpl{}
	return result
}

type decodersImpl struct {
	concurrenz.CopyOnWriteMap[int32, Decoder]
}

func (self *decodersImpl) Decode(data []byte) (Command, error) {
	if len(data) < 4 {
		return nil, errors.New("invalid command, has length less than 4")
	}
	cmdType := int32(binary.BigEndian.Uint32(data[0:4]))
	m := self.Get(cmdType)
	if m == nil {
		return nil, errors.Errorf("invalid command type: %v", cmdType)
	}
	return m.Decode(cmdType, data[4:])
}

func (self *decodersImpl) Register(key int32, value Decoder) {
	if self.Get(key) != nil {
		panic(errors.Errorf("duplicate decoder registration for key [%v]", key))
	}

	self.Put(key, value)
}

func (self *decodersImpl) RegisterF(key int32, value DecoderF) {
	self.Register(key, value)
}

func (self *decodersImpl) GetDecoder(key int32) Decoder {
	return self.Get(key)
}
