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

package model

import (
	"fmt"
)

type Path struct {
	Nodes                []*Router
	Links                []*Link
	IngressId            string
	EgressId             string
	InitiatorLocalAddr   string
	InitiatorRemoteAddr  string
	TerminatorLocalAddr  string
	TerminatorRemoteAddr string
}

func (self *Path) Cost(minRouterCost uint16) int64 {
	var cost int64
	for _, l := range self.Links {
		cost += l.GetCost()
	}
	for _, r := range self.Nodes {
		cost += int64(max(r.Cost, minRouterCost))
	}
	return cost
}

func (self *Path) String() string {
	if len(self.Nodes) < 1 {
		return "{}"
	}
	if len(self.Links) != len(self.Nodes)-1 {
		return "{malformed}"
	}
	out := fmt.Sprintf("[r/%s]", self.Nodes[0].Id)
	for i := 0; i < len(self.Links); i++ {
		out += fmt.Sprintf("->[l/%s]", self.Links[i].Id)
		out += fmt.Sprintf("->[r/%s]", self.Nodes[i+1].Id)
	}
	return out
}

func (self *Path) EqualPath(other *Path) bool {
	if len(self.Nodes) != len(other.Nodes) {
		return false
	}
	if len(self.Links) != len(other.Links) {
		return false
	}
	for i := 0; i < len(self.Nodes); i++ {
		if self.Nodes[i] != other.Nodes[i] {
			return false
		}
	}
	for i := 0; i < len(self.Links); i++ {
		if self.Links[i] != other.Links[i] {
			return false
		}
	}
	return true
}

func (self *Path) EgressRouter() *Router {
	if len(self.Nodes) > 0 {
		return self.Nodes[len(self.Nodes)-1]
	}
	return nil
}

func (self *Path) UsesLink(l *Link) bool {
	if self.Links != nil {
		for _, o := range self.Links {
			if o == l {
				return true
			}
		}
	}
	return false
}
