/*
	(c) Copyright NetFoundry Inc.

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

package xlink

import "fmt"

// LinkKey is the structured identity of a link. Two endpoints establish
// the same link iff they compute the same key.
//
// String form is "<dialerBinding>-><protocol>:<destId>-><listenerBinding>",
// with empty bindings rendered as "default" for stable map keying.
type LinkKey struct {
	DialerBinding   string
	Protocol        string
	DestId          string
	ListenerBinding string
}

// String renders the canonical key used as a map key by the link
// registry. Empty bindings are normalized to "default".
func (k LinkKey) String() string {
	db := k.DialerBinding
	if db == "" {
		db = "default"
	}
	lb := k.ListenerBinding
	if lb == "" {
		lb = "default"
	}
	return fmt.Sprintf("%s->%s:%s->%s", db, k.Protocol, k.DestId, lb)
}
