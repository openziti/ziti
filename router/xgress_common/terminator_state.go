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

package xgress_common

import "time"

// EstablishmentTimeout is how long a terminator establish or remove may take before the router
// treats it as congestion rather than success. It gates the rate-limit signal and the re-send of
// stalled attempts. Kept above normal latency so a healthy system won't trip it.
//
// Shared by the sdk-hosted and router-hosted registries on purpose: both resolve their rate limit
// controls into the same limiter, so a threshold that differed between them would feed one window
// inconsistent signals.
//
// Must not exceed the limiter's own expiry timeout (ctrl.rateLimiter.timeout). Past that, the
// limiter reclaims outstanding work as a backoff before the router can classify it, and slow but
// successful operations get recorded as congestion depending on sweep timing.
const EstablishmentTimeout = 30 * time.Second

type TerminatorState int

const (
	TerminatorStateEstablishing TerminatorState = 1
	TerminatorStateEstablished  TerminatorState = 2
	TerminatorStateDeleting     TerminatorState = 3
)

func (self TerminatorState) String() string {
	switch self {
	case TerminatorStateEstablishing:
		return "establishing"
	case TerminatorStateEstablished:
		return "established"
	case TerminatorStateDeleting:
		return "deleting"
	default:
		return "unknown"
	}
}

func (self TerminatorState) IsWorkRequired() bool {
	switch self {
	case TerminatorStateEstablishing:
		return true
	case TerminatorStateDeleting:
		return true
	case TerminatorStateEstablished:
		return false
	default:
		return false
	}
}
