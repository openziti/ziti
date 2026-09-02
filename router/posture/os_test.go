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
	"testing"

	"github.com/openziti/sdk-golang/v2/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
)

// These subtests mirror TestPostureCheckModelOs_Evaluate in
// controller/model/posture_check_model_os_test.go against the router's evaluator. Same check
// definition, same reported posture, same expected result. Each subtest names the controller subtest
// it mirrors, as of aug-26-2026. Where the router disagrees with the controller, the mirror fails.

const (
	os1Type    = "Windows"
	os1Version = "10.5.19041"

	os2Type     = "Linux"
	os2Version1 = "3.4.5"
	os2Version2 = ">=7.8.9"

	os3Type = "Android"
)

// Returns an os check and posture data that will pass with matching os type and version. Can be
// altered to test various pass/fail states. As of aug-26-2026 mirrors newMatchingOsCheckAndData in
// controller/model/posture_check_model_os_test.go.
func newMatchingOsCheckAndData() (*OsCheck, *InstanceData) {
	postureCheckId := "30qhj45"

	instanceData := &InstanceData{
		Os: &edge_client_pb.PostureResponse_Os{
			Os: &edge_client_pb.PostureResponse_OperatingSystem{
				Type:    os1Type,
				Version: os1Version,
			},
		},
	}

	osCheck := &OsCheck{
		DataState_PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{
			Id:   postureCheckId,
			Name: "os-check",
		},
		DataState_PostureCheck_OsList: &edge_ctrl_pb.DataState_PostureCheck_OsList{
			OsList: []*edge_ctrl_pb.DataState_PostureCheck_Os{
				{
					OsType:     os1Type,
					OsVersions: []string{os1Version},
				},
				{
					OsType: os2Type,
					OsVersions: []string{
						os2Version1,
						os2Version2,
					},
				},
				{
					OsType:     os3Type,
					OsVersions: nil,
				},
			},
		},
	}

	return osCheck, instanceData
}

func TestPostureCheckRouterOs_Evaluate(t *testing.T) {

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns true for exactly matching
	// valid os type and version"
	t.Run("returns true for exactly matching valid os type and version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns true for valid os type and
	// is higher than the min and lower than max"
	t.Run("returns true for valid os type and is higher than the min and lower than max", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = os2Type
		instanceData.Os.Os.Version = "8.0.0"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns true for valid os type and
	// is the min and lower than max"
	t.Run("returns true for valid os type and is the min and lower than max", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = os2Type
		instanceData.Os.Os.Version = "7.8.9"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns true for valid os type and
	// higher than min and is the max"
	t.Run("returns true for valid os type and higher than min and is the max", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = os2Type
		instanceData.Os.Os.Version = "9.9.9"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns true for valid os type and
	// higher than required major version"
	t.Run("returns true for valid os type and higher than required major version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = os2Type
		instanceData.Os.Os.Version = "8.8.9"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns true for valid os type and
	// higher than required minor version"
	t.Run("returns true for valid os type and higher than required minor version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = os2Type
		instanceData.Os.Os.Version = "7.9.9"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns true for valid os type and
	// higher than required patch version"
	t.Run("returns true for valid os type and higher than required patch version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = os2Type
		instanceData.Os.Os.Version = "7.8.10"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns false for valid os type and
	// lower than min"
	t.Run("returns false for valid os type and lower than min", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = os2Type
		instanceData.Os.Os.Version = "1.0.0"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns false for valid os type and
	// lower than required major version"
	t.Run("returns false for valid os type and lower than required major version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = os2Type
		instanceData.Os.Os.Version = "6.8.9"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns false for valid os type and
	// lower than required minor version"
	t.Run("returns false for valid os type and lower than required minor version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = os2Type
		instanceData.Os.Os.Version = "7.7.9"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns false for valid os type and
	// lower than required patch version"
	t.Run("returns false for valid os type and lower than required patch version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = os2Type
		instanceData.Os.Os.Version = "7.8.8"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns true for lower exact version
	// match and later and higher match"
	t.Run("returns true for lower exact version match and later and higher match", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = os2Type
		instanceData.Os.Os.Version = os2Version1

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns false for invalid os but
	// exactly matching version"
	t.Run("returns false for invalid os but exactly matching version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = "macOS"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns false for valid os and no
	// matching version"
	t.Run("returns false for valid os and no matching version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Version = "1.0.0"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns true for valid os and no
	// required version and no submitted version"
	t.Run("returns true for valid os and no required version and no submitted version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = os3Type
		instanceData.Os.Os.Version = ""

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns true for valid os and no
	// required version and submitted version"
	t.Run("returns true for valid os and no required version and submitted version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = os3Type
		instanceData.Os.Os.Version = "1.2.3"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// No controller equivalent. The controller lowercases the reported os type before lookup
	// (getValidOses), the router folds case in its comparison, so both accept the casing the SDK
	// actually reports.
	t.Run("returns true for valid os type with mismatched case", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Type = "windows"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.Nil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns false for posture data with
	// valid os and partial os major version match"
	t.Run("returns false for posture data with valid os and partial os major version match", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Version = "10"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns false for posture data with
	// valid os and partial os major and minor version match"
	t.Run("returns false for posture data with valid os and partial os major and minor version match", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Version = "10.5"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns false for posture data with
	// valid os and partial os major version match with dangling period"
	t.Run("returns false for posture data with valid os and partial os major version match with dangling period", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Version = "10."

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns false for posture data with
	// valid os and empty version"
	t.Run("returns false for posture data with valid os and empty version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Version = ""

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns false for posture data with
	// valid os and partially valid and mangled version"
	t.Run("returns false for posture data with valid os and partially valid and mangled version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Version = "10.5.blah"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// as of aug-26-2026 mirrors TestPostureCheckModelOs_Evaluate "returns false for posture data with
	// valid os and fully mangled version"
	t.Run("returns false for posture data with valid os and fully mangled version", func(t *testing.T) {
		osCheck, instanceData := newMatchingOsCheckAndData()
		instanceData.Os.Os.Version = "not-a-version"

		result := osCheck.Evaluate(instanceData)

		req := require.New(t)
		req.NotNil(result)
	})

	// No controller equivalent. The controller evaluates against a PostureData struct that always has
	// an Os field, the router against posture state that may not have been reported yet.
	t.Run("returns false if no posture data has been submitted", func(t *testing.T) {
		osCheck, _ := newMatchingOsCheckAndData()

		result := osCheck.Evaluate(&InstanceData{})

		req := require.New(t)
		req.NotNil(result)
		req.Equal(NilStateError, result.Cause)
	})

	// No controller equivalent, same reason as above.
	t.Run("returns false for nil posture data", func(t *testing.T) {
		osCheck, _ := newMatchingOsCheckAndData()

		result := osCheck.Evaluate(nil)

		req := require.New(t)
		req.NotNil(result)
		req.Equal(NilStateError, result.Cause)
	})
}
