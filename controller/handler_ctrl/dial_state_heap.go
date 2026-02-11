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

package handler_ctrl

type dialStateHeap []*routerDialState

func (self dialStateHeap) Len() int {
	return len(self)
}

func (self dialStateHeap) Less(i, j int) bool {
	return self[i].nextDial.Before(self[j].nextDial)
}

func (self dialStateHeap) Swap(i, j int) {
	tmp := self[i]
	self[i] = self[j]
	self[j] = tmp
}

func (self *dialStateHeap) Push(x any) {
	*self = append(*self, x.(*routerDialState))
}

func (self *dialStateHeap) Pop() any {
	old := *self
	n := len(old)
	item := old[n-1]
	*self = old[0 : n-1]
	return item
}
