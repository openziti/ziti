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

type RouterMessagingState struct {
	RouterUpdates         []*RouterUpdates         `json:"routerUpdates"`
	TerminatorValidations []*TerminatorValidations `json:"terminatorValidations"`
}

type RouterInfo struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type RouterUpdates struct {
	Router         RouterInfo   `json:"router"`
	Version        uint32       `json:"version"`
	ChangedRouters []RouterInfo `json:"changedRouters"`
	SendInProgress bool         `json:"sendInProgress"`
}

type TerminatorValidations struct {
	Router          RouterInfo `json:"router"`
	Terminators     []string   `json:"terminators"`
	CheckInProgress bool       `json:"checkInProgress"`
	LastSend        string     `json:"lastSend"`
}
