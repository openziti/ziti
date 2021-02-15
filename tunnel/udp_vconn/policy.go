/*
	Copyright NetFoundry, Inc.

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

package udp_vconn

import (
	"time"
)

func NewUnlimitedConnectionPolicy() NewConnPolicy {
	return unlimitedConnections{}
}

type unlimitedConnections struct{}

func (policy unlimitedConnections) NewConnection(count uint32) NewConnAcceptResult {
	return Allow
}

func NewLimitedConnectionPolicyDropNew(limit uint32) NewConnPolicy {
	return &limitedConnections{result: Deny, limit: limit}
}

func NewLimitedConnectionPolicyDropLRU(limit uint32) NewConnPolicy {
	return &limitedConnections{result: AllowDropLRU, limit: limit}
}

type limitedConnections struct {
	result NewConnAcceptResult
	limit  uint32
}

func (policy *limitedConnections) NewConnection(count uint32) NewConnAcceptResult {
	if count >= policy.limit {
		return policy.result
	}
	return Allow
}

func NewDefaultExpirationPolicy() ConnExpirationPolicy {
	return defaultExpirationPolicy{}
}

type defaultExpirationPolicy struct {
}

func (policy defaultExpirationPolicy) IsExpired(now, lastUsed time.Time) bool {
	return now.Sub(lastUsed) > time.Minute*5
}

func (policy defaultExpirationPolicy) PollFrequency() time.Duration {
	return time.Second * 30
}
