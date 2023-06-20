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

import (
	"time"
)

type LinksInspectResult struct {
	Links        []*LinkInspectDetail `json:"links"`
	Destinations []*LinkDest          `json:"destinations"`
	Errors       []string             `json:"errors"`
}

type LinkInspectDetail struct {
	Id          string `json:"id"`
	Key         string `json:"key"`
	Split       bool   `json:"split"`
	Protocol    string `json:"protocol"`
	DialAddress string `json:"dialAddress"`
	Dest        string `json:"dest"`
	DestVersion string `json:"destVersion"`
}

type LinkDest struct {
	Id             string       `json:"id"`
	Version        string       `json:"version"`
	Healthy        bool         `json:"healthy"`
	UnhealthySince *time.Time   `json:"unhealthySince,omitempty"`
	LinkStates     []*LinkState `json:"linkStates"`
}

type LinkState struct {
	Id             string   `json:"id"`
	Key            string   `json:"key"`
	Status         string   `json:"status"`
	DialAttempts   uint     `json:"dialAttempts"`
	ConnectedCount uint     `json:"connectedCount"`
	RetryDelay     string   `json:"retryDelay"`
	NextDial       string   `json:"nextDial"`
	TargetAddress  string   `json:"targetAddress"`
	TargetGroups   []string `json:"targetGroups"`
	TargetBinding  string   `json:"targetBinding"`
	DialerGroups   []string `json:"dialerGroups"`
	DialerBinding  string   `json:"dialerBinding"`
}
