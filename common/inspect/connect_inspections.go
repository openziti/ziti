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

package inspect

type RouterIdentityConnections struct {
	IdentityConnections map[string]*RouterIdentityConnectionDetail `json:"identity_connections"`
	LastFullSync        string                                     `json:"last_full_sync"`
	QueuedEventCount    int64                                      `json:"queued_event_count"`
	NeedFullSync        []string                                   `json:"need_full_sync"`
}

type RouterIdentityConnectionDetail struct {
	UnreportedCount           uint64                    `json:"unreported_count"`
	UnreportedStateChanged    bool                      `json:"unreported_state_changed"`
	BeingReportedCount        uint64                    `json:"being_reported_count"`
	BeingReportedStateChanged bool                      `json:"being_reported_state_changed"`
	Connections               []*RouterConnectionDetail `json:"connections"`
}

type RouterConnectionDetail struct {
	Id      string `json:"id"`
	Closed  bool   `json:"closed"`
	SrcAddr string `json:"srcAddr"`
	DstAddr string `json:"dstAddr"`
}

type CtrlIdentityConnections struct {
	Connections map[string]*CtrlIdentityConnectionDetail `json:"connections"`
}

type CtrlIdentityConnectionDetail struct {
	ConnectedRouters  map[string]*CtrlRouterConnection `json:"connected_routers"`
	LastReportedState string                           `json:"last_reported_state"`
}

type CtrlRouterConnection struct {
	RouterId           string `json:"router_id"`
	Closed             bool   `json:"closed"`
	TimeSinceLastWrite string `json:"time_since_last_write"`
}
