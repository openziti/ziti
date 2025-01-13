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

package event

import (
	"time"
)

type ConnectSource string
type ConnectDestination string

const (
	ConnectEventNS                      = "connect"
	ConnectSourceRouter   ConnectSource = "router"
	ConnectSourcePeer     ConnectSource = "peer"
	ConnectSourceIdentity ConnectSource = "identity"

	ConnectDestinationController ConnectDestination = "ctrl"
	ConnectDestinationRouter     ConnectDestination = "router"
)

type ConnectEvent struct {
	Namespace  string             `json:"namespace"`
	EventSrcId string             `json:"event_src_id"`
	SrcType    ConnectSource      `json:"src_type"`
	DstType    ConnectDestination `json:"dst_type"`
	SrcId      string             `json:"src_id"`
	SrcAddr    string             `json:"src_addr"`
	DstId      string             `json:"dst_id"`
	DstAddr    string             `json:"dst_addr"`
	Timestamp  time.Time          `json:"timestamp"`
}

type ConnectEventHandler interface {
	AcceptConnectEvent(event *ConnectEvent)
}

type ConnectEventHandlerWrapper interface {
	ConnectEventHandler
	IsWrapping(value ConnectEventHandler) bool
}
