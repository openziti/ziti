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

package predicate

import (
	"gopkg.in/Masterminds/squirrel.v1"
)

type parseState struct {
	root         *group
	currentGroup *group
}

func newParseState() *parseState {
	r := newGroup(nil)
	return &parseState{
		root:         r,
		currentGroup: r,
	}
}

func (ps *parseState) AddOp(o resolver) {
	ps.currentGroup.operations = append(ps.currentGroup.operations, o)
}

func (ps *parseState) AddOr() {
	ps.currentGroup.conjugations = append(ps.currentGroup.conjugations, newOr())
}

func (ps *parseState) AddAnd() {
	ps.currentGroup.conjugations = append(ps.currentGroup.conjugations, newAnd())
}

func (ps *parseState) EnterGroup() {
	ng := newGroup(ps.currentGroup)
	ps.currentGroup.operations = append(ps.currentGroup.operations, ng)
	ps.currentGroup = ng
}

func (ps *parseState) ExitGroup() {
	ps.currentGroup = ps.currentGroup.Parent
}

func (ps *parseState) End() (squirrel.Sqlizer, IdentifierOps) {
	return ps.root.Resolve()
}
