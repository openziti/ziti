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

package agent

import (
	"encoding/json"
	"io"
	"net"
)

// AppInfoV2Response is the JSON body returned by the AppInfoV2 command. It
// carries the same identity fields as the legacy AppInfo command plus the two
// capability lists.
type AppInfoV2Response struct {
	Type              string   `json:"type,omitempty"`
	Id                string   `json:"id,omitempty"`
	Alias             string   `json:"alias,omitempty"`
	Version           string   `json:"version,omitempty"`
	AgentCapabilities []string `json:"agent_capabilities,omitempty"`
	AppCapabilities   []string `json:"app_capabilities,omitempty"`
}

// ReadAppInfoV2Response reads an AppInfoV2 response from conn. A zero-byte read
// (clean EOF) means the server has no AppInfoV2 handler, i.e. it predates the
// command; in that case it returns (nil, false, nil) so the caller can fall
// back to the legacy AppInfo command and treat the capability set as empty.
func ReadAppInfoV2Response(conn net.Conn) (*AppInfoV2Response, bool, error) {
	data, err := io.ReadAll(conn)
	if err != nil {
		return nil, false, err
	}
	if len(data) == 0 {
		return nil, false, nil
	}
	resp := &AppInfoV2Response{}
	if err := json.Unmarshal(data, resp); err != nil {
		return nil, false, err
	}
	return resp, true, nil
}
