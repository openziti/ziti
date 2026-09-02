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

package posture

import (
	"strings"
	"testing"

	"github.com/openziti/sdk-golang/v2/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/stretchr/testify/require"
)

// These subtests mirror TestPostureCheckModelProcessMulti_Evaluate and
// TestPostureCheckModelProcess_Evaluate in controller/model against the router's evaluator. Same check
// definition, same reported posture, same expected result. Each subtest names the controller subtest it
// mirrors, as of aug-26-2026. Where the router disagrees with the controller, the mirror fails.
//
// The fixtures report no OS posture. That is what a client sends when the only check on the policy is a
// process check: the SDK collects OS posture only when some service's posture query set asks for it.
// Neither controller process evaluator reads OsType.

const (
	procPath       = `C:\some\path\some.exe`
	procBinaryHash = "b4f3228217a2bae3f21f6b6df3750d0723a5c3973db9aad360a8f25bc31e3676d38180cf0abc89d7fca7a26e1919a1e52739ed3116011acc7e96630313da56b8"
	procSigner     = "950248b9e8b0dd41938018a871a13dd92bed4614"
)

// Returns a process multi check and posture data that will pass with matching path, hash, and signer
// fingerprint. Can be altered to test various pass/fail states. As of aug-26-2026 mirrors
// newMatchingProcessMultiCheckAndData in controller/model/posture_check_model_process_multi_test.go.
func newMatchingProcessMultiCheckAndData() (*ProcessCheck, *InstanceData) {
	postureCheckId := "30qhj45"

	instanceData := &InstanceData{
		ProcessList: &edge_client_pb.PostureResponse_ProcessList{
			Processes: []*edge_client_pb.PostureResponse_Process{
				{
					Path:               procPath,
					IsRunning:          true,
					Hash:               procBinaryHash,
					SignerFingerprints: []string{procSigner},
				},
			},
		},
	}

	processCheck := &ProcessCheck{
		DataState_PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{
			Id:   postureCheckId,
			Name: "process-multi-check",
		},
		DataState_PostureCheck_ProcessMulti: &edge_ctrl_pb.DataState_PostureCheck_ProcessMulti{
			Semantic: db.SemanticAllOf,
			Processes: []*edge_ctrl_pb.DataState_PostureCheck_Process{
				{
					OsType: os1Type,
					Path:   procPath,
					Hashes: []string{
						"something that will never match 1",
						procBinaryHash,
						"something that will never match 2",
					},
					Fingerprints: []string{
						"something that will never match 1",
						procSigner,
						"something that will never match 2",
					},
				},
			},
		},
	}

	return processCheck, instanceData
}

// Returns a single process check and posture data that will pass. The check is built through
// CtrlCheckToLogic, the way the router builds it from the router data model, because that conversion is
// where the controller's optional signer fingerprint becomes a one element list. As of aug-26-2026
// mirrors newMatchingProcessCheckAndData in controller/model/posture_check_model_process_test.go.
func newMatchingProcessCheckAndData(signerFingerprint string) (Checker, *InstanceData) {
	instanceData := &InstanceData{
		ProcessList: &edge_client_pb.PostureResponse_ProcessList{
			Processes: []*edge_client_pb.PostureResponse_Process{
				{
					Path:               procPath,
					IsRunning:          true,
					Hash:               procBinaryHash,
					SignerFingerprints: []string{procSigner},
				},
			},
		},
	}

	check := CtrlCheckToLogic(&edge_ctrl_pb.DataState_PostureCheck{
		Id:   "30qhj45",
		Name: "process-check",
		Subtype: &edge_ctrl_pb.DataState_PostureCheck_Process_{
			Process: &edge_ctrl_pb.DataState_PostureCheck_Process{
				OsType:       os1Type,
				Path:         procPath,
				Hashes:       []string{procBinaryHash},
				Fingerprints: []string{signerFingerprint},
			},
		},
	})

	return check, instanceData
}

func TestPostureCheckRouterProcessMulti_Evaluate(t *testing.T) {

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns true for valid id,
	// running, hash, and fingerprint"
	t.Run("returns true for valid id, running, hash, and fingerprint", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns false if not
	// running"
	t.Run("returns false if not running", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()
		instanceData.ProcessList.Processes[0].IsRunning = false

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns true for valid id,
	// running, hash, and fingerprint with mismatched hash case"
	t.Run("returns true for valid id, running, hash, and fingerprint with mismatched hash case", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()
		instanceData.ProcessList.Processes[0].Hash = strings.ToUpper(instanceData.ProcessList.Processes[0].Hash)

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns true for valid id,
	// running, hash, and fingerprint with mismatched signer case"
	t.Run("returns true for valid id, running, hash, and fingerprint with mismatched signer case", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()
		instanceData.ProcessList.Processes[0].SignerFingerprints[0] = strings.ToUpper(instanceData.ProcessList.Processes[0].SignerFingerprints[0])

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns true for valid id
	// and running but check has null hashes and no signer"
	t.Run("returns true for valid id and running but check has null hashes and no signer", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()
		processCheck.Processes[0].Hashes = nil
		processCheck.Processes[0].Fingerprints = nil

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns true for valid id
	// and running but check has empty hashes and no signer"
	t.Run("returns true for valid id and running but check has empty hashes and no signer", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()
		processCheck.Processes[0].Hashes = []string{}
		processCheck.Processes[0].Fingerprints = []string{}

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns true for valid id,
	// running, signer but check has no hash"
	t.Run("returns true for valid id, running, signer but check has no hash", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()
		processCheck.Processes[0].Hashes = nil

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns true for valid id,
	// running, hashes but check has no signer"
	t.Run("returns true for valid id, running, hashes but check has no signer", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()
		processCheck.Processes[0].Fingerprints = nil

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns false if paths do
	// not match"
	t.Run("returns false if paths do not match", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()
		instanceData.ProcessList.Processes[0].Path = `C:\some\other\path\other.exe`

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns false if
	// signerByIssuer do not match"
	t.Run("returns false if signerByIssuer do not match", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()
		instanceData.ProcessList.Processes[0].SignerFingerprints = []string{"a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"}

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns false if hashes do
	// not match"
	t.Run("returns false if hashes do not match", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()
		instanceData.ProcessList.Processes[0].Hash = "0000000000000000000000000000000000000000000000000000000000000000"

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns false not running,
	// invalid hash, invalid signer"
	t.Run("returns false not running, invalid hash, invalid signer", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()
		instanceData.ProcessList.Processes[0].IsRunning = false
		instanceData.ProcessList.Processes[0].Hash = "0000000000000000000000000000000000000000000000000000000000000000"
		instanceData.ProcessList.Processes[0].SignerFingerprints = []string{"a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"}

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns true for AnyOf when
	// one of two processes matches"
	t.Run("returns true for AnyOf when one of two processes matches", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()
		processCheck.Semantic = db.SemanticAnyOf
		processCheck.Processes = append([]*edge_ctrl_pb.DataState_PostureCheck_Process{
			{
				OsType: os2Type,
				Path:   "/usr/bin/never-running",
			},
		}, processCheck.Processes...)

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcessMulti_Evaluate "returns false if the
	// required process was never reported"
	t.Run("returns false if the required process was never reported", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessMultiCheckAndData()
		instanceData.ProcessList.Processes = nil

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// No controller equivalent. The controller evaluates against a PostureData struct that always
	// exists, the router against posture state that may not have been reported yet.
	t.Run("returns false if no posture data has been submitted", func(t *testing.T) {
		processCheck, _ := newMatchingProcessMultiCheckAndData()

		result := processCheck.Evaluate(&InstanceData{})

		req := require.New(t)
		req.NotNil(result)
	})

	// No controller equivalent, same reason as above.
	t.Run("returns false for nil posture data", func(t *testing.T) {
		processCheck, _ := newMatchingProcessMultiCheckAndData()

		result := processCheck.Evaluate(nil)

		req := require.New(t)
		req.NotNil(result)
		req.Equal(NilStateError, result.Cause)
	})
}

func TestPostureCheckRouterProcess_Evaluate(t *testing.T) {

	// as of aug-26-2026 mirrors TestPostureCheckModelProcess_Evaluate "returns true for valid id,
	// running, hash, and fingerprint"
	t.Run("returns true for valid id, running, hash, and fingerprint", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessCheckAndData(procSigner)

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcess_Evaluate "returns true for valid id and
	// running but check has null hashes and no signer"
	t.Run("returns true for valid id and running but check has no signer", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessCheckAndData("")

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcess_Evaluate "returns true for valid id and
	// running but check has empty hashes and no signer"
	t.Run("returns true for valid id and running, check has no signer, and none is reported", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessCheckAndData("")
		instanceData.ProcessList.Processes[0].SignerFingerprints = nil

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcess_Evaluate "returns true for valid id,
	// running, hash, and fingerprint with mismatched hash case"
	t.Run("returns true for valid id, running, hash, and fingerprint with mismatched hash case", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessCheckAndData(procSigner)
		instanceData.ProcessList.Processes[0].Hash = strings.ToUpper(instanceData.ProcessList.Processes[0].Hash)

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcess_Evaluate "returns true for valid id,
	// running, hash, and fingerprint with mismatched signer case"
	t.Run("returns true for valid id, running, hash, and fingerprint with mismatched signer case", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessCheckAndData(strings.ToUpper(procSigner))

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcess_Evaluate "returns false if ids do not
	// match"
	t.Run("returns false if ids do not match", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessCheckAndData(procSigner)
		instanceData.ProcessList.Processes[0].Path = `C:\some\other\path\other.exe`

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcess_Evaluate "returns false if hashes do not
	// match"
	t.Run("returns false if hashes do not match", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessCheckAndData(procSigner)
		instanceData.ProcessList.Processes[0].Hash = "0000000000000000000000000000000000000000000000000000000000000000"

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcess_Evaluate "returns false if signerByIssuer
	// do not match"
	t.Run("returns false if signerByIssuer do not match", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessCheckAndData(procSigner)
		instanceData.ProcessList.Processes[0].SignerFingerprints = []string{"a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"}

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcess_Evaluate "returns false if not running"
	t.Run("returns false if not running", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessCheckAndData(procSigner)
		instanceData.ProcessList.Processes[0].IsRunning = false

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelProcess_Evaluate "returns false not running,
	// invalid hash, invalid signer"
	t.Run("returns false not running, invalid hash, invalid signer", func(t *testing.T) {
		processCheck, instanceData := newMatchingProcessCheckAndData(procSigner)
		instanceData.ProcessList.Processes[0].IsRunning = false
		instanceData.ProcessList.Processes[0].Hash = "0000000000000000000000000000000000000000000000000000000000000000"
		instanceData.ProcessList.Processes[0].SignerFingerprints = []string{"a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"}

		result := processCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})
}
