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

package sync_strats

import (
	"testing"

	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

// Test_newPostureCheck_ProcessWithoutFingerprint covers a process check the administrator
// configured no signer for. Routers treat every entry in the fingerprint list as a signer the
// client must report, and no client reports an empty one, so an unconfigured fingerprint must
// reach the router as no entries at all.
func Test_newPostureCheck_ProcessWithoutFingerprint(t *testing.T) {
	req := require.New(t)

	stored := &db.PostureCheck{
		BaseExtEntity: boltz.BaseExtEntity{Id: "pc1"},
		Name:          "proc",
		TypeId:        db.PostureCheckTypeProcess,
		SubType: &db.PostureCheckProcess{
			OperatingSystem: "Windows",
			Path:            "C:\\Windows\\System32\\notepad.exe",
			Fingerprint:     "",
		},
	}

	result := newPostureCheck(stored)

	process := result.Subtype.(*edge_ctrl_pb.DataState_PostureCheck_Process_).Process
	req.Len(process.Fingerprints, 0)
}

// Test_newPostureCheck_ProcessWithFingerprint covers the configured case, where the single stored
// signer is the one entry the router must match against.
func Test_newPostureCheck_ProcessWithFingerprint(t *testing.T) {
	req := require.New(t)

	stored := &db.PostureCheck{
		BaseExtEntity: boltz.BaseExtEntity{Id: "pc1"},
		Name:          "proc",
		TypeId:        db.PostureCheckTypeProcess,
		SubType: &db.PostureCheckProcess{
			OperatingSystem: "Windows",
			Path:            "C:\\Windows\\System32\\notepad.exe",
			Fingerprint:     "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678",
		},
	}

	result := newPostureCheck(stored)

	process := result.Subtype.(*edge_ctrl_pb.DataState_PostureCheck_Process_).Process
	req.Equal([]string{"a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"}, process.Fingerprints)
}
