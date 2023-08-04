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

package events

import (
	"fmt"
	"time"
)

const EntityCountEventNS = "edge.entityCounts"

type EntityCountEvent struct {
	Namespace string           `json:"namespace"`
	Timestamp time.Time        `json:"timestamp"`
	Counts    map[string]int64 `json:"counts"`
	Error     string           `json:"error"`
}

func (event *EntityCountEvent) String() string {
	return fmt.Sprintf("%v timestamp=%v counts=%+v err=%v",
		event.Namespace, event.Timestamp, event.Counts, event.Error)
}

type EntityCountEventHandler interface {
	AcceptEntityCountEvent(event *EntityCountEvent)
}
