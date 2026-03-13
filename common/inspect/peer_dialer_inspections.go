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

// PeerDialerKey is the inspection key used to retrieve peer dialer state via the inspect API.
const PeerDialerKey = "ctrl-peer-dialer"

// PeerDialerInspectResult is the top-level result returned when inspecting the peer dialer.
type PeerDialerInspectResult struct {
	Config PeerDialerConfigDetail `json:"config"`
	Peers  []*PeerDialerPeerDial  `json:"peers"`
}

// PeerDialerConfigDetail contains the active configuration of the peer dialer.
type PeerDialerConfigDetail struct {
	MinRetryInterval   string  `json:"minRetryInterval"`
	MaxRetryInterval   string  `json:"maxRetryInterval"`
	RetryBackoffFactor float64 `json:"retryBackoffFactor"`
	FastFailureWindow  string  `json:"fastFailureWindow"`
}

// PeerDialerPeerDial describes the current dial state for a single peer.
type PeerDialerPeerDial struct {
	Address      string `json:"address"`
	Status       string `json:"status"`
	DialAttempts uint32 `json:"dialAttempts"`
	RetryDelay   string `json:"retryDelay"`
	NextDial     string `json:"nextDial"`
}
