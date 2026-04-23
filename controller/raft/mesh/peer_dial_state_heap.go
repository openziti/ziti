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

package mesh

type peerDialStateHeap []*peerDialState

func (self peerDialStateHeap) Len() int {
	return len(self)
}

func (self peerDialStateHeap) Less(i, j int) bool {
	return self[i].nextDial.Before(self[j].nextDial)
}

func (self peerDialStateHeap) Swap(i, j int) {
	self[i], self[j] = self[j], self[i]
	self[i].heapIndex = i
	self[j].heapIndex = j
}

func (self *peerDialStateHeap) Push(x any) {
	s := x.(*peerDialState)
	s.heapIndex = len(*self)
	*self = append(*self, s)
}

func (self *peerDialStateHeap) Pop() any {
	old := *self
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.heapIndex = -1
	*self = old[0 : n-1]
	return item
}
