/*
	Copyright NetFoundry, Inc.

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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/inspect"
	"sync/atomic"
)

type LinkReceiveBuffer struct {
	tree               *btree.Tree
	sequence           int32
	maxSequence        int32
	size               uint32
	lastBufferSizeSent uint32
}

func NewLinkReceiveBuffer() *LinkReceiveBuffer {
	return &LinkReceiveBuffer{
		tree:     btree.NewWith(10240, utils.Int32Comparator),
		sequence: -1,
	}
}

func (buffer *LinkReceiveBuffer) Size() uint32 {
	return atomic.LoadUint32(&buffer.size)
}

func (buffer *LinkReceiveBuffer) ReceiveUnordered(payload *Payload, maxSize uint32) bool {
	if payload.GetSequence() <= buffer.sequence {
		return true
	}

	if buffer.size > maxSize && payload.Sequence > buffer.maxSequence {
		droppedPayloadsMeter.Mark(1)
		return false
	}

	treeSize := buffer.tree.Size()
	buffer.tree.Put(payload.GetSequence(), payload)
	if buffer.tree.Size() > treeSize {
		payloadSize := len(payload.Data)
		size := atomic.AddUint32(&buffer.size, uint32(payloadSize))
		pfxlog.Logger().Tracef("Payload %v of size %v added to transmit buffer. New size: %v", payload.Sequence, payloadSize, size)
		if payload.Sequence > buffer.maxSequence {
			buffer.maxSequence = payload.Sequence
		}
	}
	return true
}

func (buffer *LinkReceiveBuffer) PeekHead() *Payload {
	if val := buffer.tree.LeftValue(); val != nil {
		payload := val.(*Payload)
		if payload.Sequence == buffer.sequence+1 {
			return payload
		}
	}
	return nil
}

func (buffer *LinkReceiveBuffer) Remove(payload *Payload) {
	buffer.tree.Remove(payload.Sequence)
	buffer.sequence = payload.Sequence
}

func (buffer *LinkReceiveBuffer) getLastBufferSizeSent() uint32 {
	return atomic.LoadUint32(&buffer.lastBufferSizeSent)
}

func (buffer *LinkReceiveBuffer) Inspect() *inspect.XgressRecvBufferDetail {
	return &inspect.XgressRecvBufferDetail{
		Size:         buffer.Size(),
		LastSizeSent: buffer.getLastBufferSizeSent(),
	}
}
