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

type LinkInspectDetail struct {
	Id                 string            `json:"id"`
	Iteration          uint32            `json:"iteration"`
	Key                string            `json:"key"`
	Split              bool              `json:"split"`
	Protocol           string            `json:"protocol"`
	DialAddress        string            `json:"dialAddress"`
	Dest               string            `json:"dest"`
	DestVersion        string            `json:"destVersion"`
	Dialed             bool              `json:"dialed"`
	Underlays          map[string]int    `json:"underlays"`
	Connections        []*LinkConnection `json:"connections"`
	ConnStateIteration uint32            `json:"connStateIteration"`
}

type LinksInspectResult struct {
	Links        []*LinkInspectDetail `json:"links"`
	Destinations []*LinkDest          `json:"destinations"`
	Errors       []string             `json:"errors"`
}

type LinkDest struct {
	Id      string `json:"id"`
	Version string `json:"version"`
	// Healthy is the union of every health report the router has received for this destination, including
	// reports from controllers it is no longer connected to. It selects the dial backoff.
	Healthy        bool       `json:"healthy"`
	UnhealthySince *time.Time `json:"unhealthySince,omitempty"`
	// LastAffirmedAt is the last time a controller the router was connected to said this destination was up.
	// Unlike Healthy it can lapse, and once it has lapsed for long enough the router stops tracking the
	// destination. A destination reading healthy with an old LastAffirmedAt is one whose only vouching
	// controller has gone away.
	LastAffirmedAt *time.Time   `json:"lastAffirmedAt,omitempty"`
	LinkStates     []*LinkState `json:"linkStates"`
}

type LinkConnection struct {
	Type   string
	Source string
	Dest   string
}

type LinkState struct {
	Id                string   `json:"id"`
	Key               string   `json:"key"`
	Status            string   `json:"status"`
	DialAttempts      uint64   `json:"dialAttempts"`
	ConnectedCount    uint64   `json:"connectedCount"`
	RetryDelay        string   `json:"retryDelay"`
	NextDial          string   `json:"nextDial"`
	TargetAddress     string   `json:"targetAddress"`
	TargetGroups      []string `json:"targetGroups"`
	TargetBinding     string   `json:"targetBinding"`
	DialerGroups      []string `json:"dialerGroups"`
	DialerBinding     string   `json:"dialerBinding"`
	CtrlsNotified     bool     `json:"ctrlsNotified"`
	EstablishedLinkId string   `json:"establishedLinkId"`
}
