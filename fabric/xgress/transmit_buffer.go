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

package xgress

import (
	"github.com/emirpasic/gods/trees/btree"
	"github.com/emirpasic/gods/utils"
)

type TransmitBuffer struct {
	tree     *btree.Tree
	sequence int32
}

func NewTransmitBuffer() *TransmitBuffer {
	return &TransmitBuffer{
		tree:     btree.NewWith(10240, utils.Int32Comparator),
		sequence: -1,
	}
}

func (buffer *TransmitBuffer) ReceiveUnordered(payload *Payload) {
	if payload.GetSequence() > buffer.sequence {
		buffer.tree.Put(payload.GetSequence(), payload)
	}
}

func (buffer *TransmitBuffer) ReadyForTransmit() []*Payload {
	var ready []*Payload

	if buffer.tree.LeftKey() != nil {
		nextSequence := buffer.tree.LeftKey().(int32)
		for nextSequence == buffer.sequence+1 {
			payload := buffer.tree.LeftValue().(*Payload)
			buffer.tree.Remove(nextSequence)
			buffer.sequence = nextSequence

			ready = append(ready, payload)

			if buffer.tree.Size() > 0 {
				nextSequence = buffer.tree.LeftKey().(int32)
			}
		}
	}

	return ready
}
