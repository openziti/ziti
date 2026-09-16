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

package handler_peer_ctrl

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/stretchr/testify/require"
)

// The peer receiving this rebuilds a typed error from these fields, so the encoding has to
// succeed and carry the application code as a string. A bound method or other non-JSON value in
// the map would make json.Marshal fail and drop the whole error to the generic path.
func Test_EncodeApiError(t *testing.T) {
	req := require.New(t)

	apiErr := errorz.NewNotFound()
	buf, err := encodeApiError(apiErr)
	req.NoError(err)

	decoded := map[string]any{}
	req.NoError(json.Unmarshal(buf, &decoded))
	req.Equal(errorz.NotFoundCode, decoded["code"])
	req.Equal(apiErr.Message, decoded["message"])
	req.EqualValues(http.StatusNotFound, decoded["status"])
	req.NotContains(decoded, "cause")

	apiErr = errorz.NewNotFound()
	apiErr.Cause = errors.New("underlying")
	buf, err = encodeApiError(apiErr)
	req.NoError(err)
	decoded = map[string]any{}
	req.NoError(json.Unmarshal(buf, &decoded))
	req.Equal("underlying", decoded["cause"], "a cause with no exported fields is sent as its text")
	req.Equal("*errors.errorString", decoded["causeType"])
}
