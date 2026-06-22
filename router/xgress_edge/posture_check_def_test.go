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

package xgress_edge

import (
	"testing"

	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
)

// Test_buildPostureCheckDef_singleProcessNormalized locks in that a single PROCESS posture check is
// translated to a one-element PROCESS_MULTI on the wire, so SDKs only ever handle one process-check
// shape and never see a bare "PROCESS" type.
func Test_buildPostureCheckDef_singleProcessNormalized(t *testing.T) {
	check := &common.PostureCheck{
		DataStatePostureCheck: &edge_ctrl_pb.DataState_PostureCheck{
			TypeId: "PROCESS",
			Subtype: &edge_ctrl_pb.DataState_PostureCheck_Process_{
				Process: &edge_ctrl_pb.DataState_PostureCheck_Process{
					OsType: "Windows",
					Path:   "C:\\Program Files\\app\\agent.exe",
				},
			},
		},
	}

	def := buildPostureCheckDef("pc-proc", check)

	require.Equal(t, "PROCESS_MULTI", def.Type, "single PROCESS must be normalized to PROCESS_MULTI on the wire")
	require.Equal(t, "AllOf", def.Semantic, "a single normalized process uses AllOf")
	require.Len(t, def.Processes, 1)
	require.Equal(t, "Windows", def.Processes[0].OsType)
	require.Equal(t, "C:\\Program Files\\app\\agent.exe", def.Processes[0].Path)
}

// Test_buildPostureCheckDef_processMultiPassthrough verifies a real PROCESS_MULTI carries its
// semantic and every process through unchanged.
func Test_buildPostureCheckDef_processMultiPassthrough(t *testing.T) {
	check := &common.PostureCheck{
		DataStatePostureCheck: &edge_ctrl_pb.DataState_PostureCheck{
			TypeId: "PROCESS_MULTI",
			Subtype: &edge_ctrl_pb.DataState_PostureCheck_ProcessMulti_{
				ProcessMulti: &edge_ctrl_pb.DataState_PostureCheck_ProcessMulti{
					Semantic: "AnyOf",
					Processes: []*edge_ctrl_pb.DataState_PostureCheck_Process{
						{OsType: "Linux", Path: "/usr/bin/a"},
						{OsType: "macOS", Path: "/usr/bin/b"},
					},
				},
			},
		},
	}

	def := buildPostureCheckDef("pc-multi", check)

	require.Equal(t, "PROCESS_MULTI", def.Type)
	require.Equal(t, "AnyOf", def.Semantic)
	require.Len(t, def.Processes, 2)
}
