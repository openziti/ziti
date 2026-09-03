package posture

import (
	"testing"

	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/stretchr/testify/require"
)

const testProcPath = "C:\\Windows\\System32\\notepad.exe"

func newProcessCheck(semantic string) *ProcessCheck {
	return &ProcessCheck{
		DataState_PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{
			Id:   "test-id",
			Name: "my-proc-multi",
		},
		DataState_PostureCheck_ProcessMulti: &edge_ctrl_pb.DataState_PostureCheck_ProcessMulti{
			Semantic: semantic,
			Processes: []*edge_ctrl_pb.DataState_PostureCheck_Process{
				{
					OsType: "Windows",
					Path:   testProcPath,
				},
			},
		},
	}
}

// Process reported at the required path but no OS posture: OS state is nil. Must fail cleanly, not panic.
func TestProcessCheck_ProcessReportedButNilOs_DoesNotPanic(t *testing.T) {
	for _, semantic := range []string{db.SemanticAnyOf, db.SemanticAllOf} {
		t.Run(semantic, func(t *testing.T) {
			check := newProcessCheck(semantic)

			data := &InstanceData{
				// Os intentionally nil: process posture was sent, OS posture was not.
				ProcessList: &edge_client_pb.PostureResponse_ProcessList{
					Processes: []*edge_client_pb.PostureResponse_Process{
						{
							Path:      testProcPath,
							IsRunning: true,
						},
					},
				},
			}

			result := check.Evaluate(data)

			if result == nil {
				t.Fatalf("expected a non-nil CheckError when OS posture data is missing, got nil")
			}
			if result.Name != "my-proc-multi" {
				t.Fatalf("expected CheckError for check 'my-proc-multi', got %q", result.Name)
			}
		})
	}
}

// No posture data at all: nil ProcessList and nil OS state. Must fail cleanly, not panic.
func TestProcessCheck_NoPostureData_DoesNotPanic(t *testing.T) {
	for _, semantic := range []string{db.SemanticAnyOf, db.SemanticAllOf} {
		t.Run(semantic, func(t *testing.T) {
			check := newProcessCheck(semantic)

			result := check.Evaluate(&InstanceData{})

			if result == nil {
				t.Fatalf("expected a non-nil CheckError when no posture data has been reported, got nil")
			}
		})
	}
}

// Nil InstanceData must fail with NilStateError, not panic.
func TestProcessCheck_NilInstanceData(t *testing.T) {
	for _, semantic := range []string{db.SemanticAnyOf, db.SemanticAllOf} {
		check := newProcessCheck(semantic)

		result := check.Evaluate(nil)

		if result == nil {
			t.Fatalf("semantic %s: expected a non-nil CheckError for nil InstanceData", semantic)
		}
		if result.Cause != NilStateError {
			t.Fatalf("semantic %s: expected NilStateError, got %v", semantic, result.Cause)
		}
	}
}

const (
	lowerHash = "3a7bd3e2360a3d29eea436fcfb7e44c735d117c42d1c1835420b6b9942dd4f1b"
	upperHash = "3A7BD3E2360A3D29EEA436FCFB7E44C735D117C42D1C1835420B6B9942DD4F1B"
	otherHash = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

	lowerPrint     = "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"
	upperPrint     = "A1B2C3D4E5F60718293A4B5C6D7E8F9012345678"
	separatedPrint = "A1:B2:C3:D4:E5:F6:07:18:29:3A:4B:5C:6D:7E:8F:90:12:34:56:78"
	otherPrint     = "0011223344556677889900aabbccddeeff001122"
)

// newProcessCheckWith builds a single-process AllOf check carrying the configured hashes and
// signer fingerprints, as the router receives them from the controller's data state.
func newProcessCheckWith(hashes, fingerprints []string) *ProcessCheck {
	check := newProcessCheck(db.SemanticAllOf)
	check.Processes[0].Hashes = hashes
	check.Processes[0].Fingerprints = fingerprints
	return check
}

// reportedProcess builds posture state for a running process at the required path with the given
// reported hash and signer fingerprints.
func reportedProcess(hash string, fingerprints []string) *InstanceData {
	return &InstanceData{
		Os: &edge_client_pb.PostureResponse_Os{
			Os: &edge_client_pb.PostureResponse_OperatingSystem{Type: "Windows"},
		},
		ProcessList: &edge_client_pb.PostureResponse_ProcessList{
			Processes: []*edge_client_pb.PostureResponse_Process{
				{
					Path:               testProcPath,
					IsRunning:          true,
					Hash:               hash,
					SignerFingerprints: fingerprints,
				},
			},
		},
	}
}

func processListResponse(hash string, fingerprints []string) *edge_client_pb.PostureResponse {
	return &edge_client_pb.PostureResponse{
		Type: &edge_client_pb.PostureResponse_ProcessList_{
			ProcessList: &edge_client_pb.PostureResponse_ProcessList{
				Processes: []*edge_client_pb.PostureResponse_Process{
					{
						Path:               testProcPath,
						IsRunning:          true,
						Hash:               hash,
						SignerFingerprints: fingerprints,
					},
				},
			},
		},
	}
}

// Test_ProcessCheck_HashCaseIsIgnored locks in that hex hashes differing only in case are the same
// hash, as the controller treats them, in both directions.
func Test_ProcessCheck_HashCaseIsIgnored(t *testing.T) {
	t.Run("configured uppercase, reported lowercase", func(t *testing.T) {
		check := newProcessCheckWith([]string{upperHash}, nil)

		require.Nil(t, check.Evaluate(reportedProcess(lowerHash, nil)))
	})

	t.Run("configured lowercase, reported uppercase", func(t *testing.T) {
		check := newProcessCheckWith([]string{lowerHash}, nil)

		require.Nil(t, check.Evaluate(reportedProcess(upperHash, nil)))
	})
}

// Test_ProcessCheck_DifferentHashFails locks in that case insensitivity does not make a genuinely
// different hash pass.
func Test_ProcessCheck_DifferentHashFails(t *testing.T) {
	check := newProcessCheckWith([]string{upperHash}, nil)

	require.NotNil(t, check.Evaluate(reportedProcess(otherHash, nil)))
}

// Test_ProcessCheck_FingerprintCaseIsIgnored locks in the same for signer fingerprints, which the
// controller lowercases on both sides before comparing.
func Test_ProcessCheck_FingerprintCaseIsIgnored(t *testing.T) {
	t.Run("configured uppercase, reported lowercase", func(t *testing.T) {
		check := newProcessCheckWith(nil, []string{upperPrint})

		require.Nil(t, check.Evaluate(reportedProcess(lowerHash, []string{lowerPrint})))
	})

	t.Run("configured lowercase, reported uppercase", func(t *testing.T) {
		check := newProcessCheckWith(nil, []string{lowerPrint})

		require.Nil(t, check.Evaluate(reportedProcess(lowerHash, []string{upperPrint})))
	})
}

// Test_ProcessCheck_DifferentFingerprintFails locks in that case insensitivity does not make an
// unrelated signer pass.
func Test_ProcessCheck_DifferentFingerprintFails(t *testing.T) {
	check := newProcessCheckWith(nil, []string{upperPrint})

	require.NotNil(t, check.Evaluate(reportedProcess(lowerHash, []string{otherPrint})))
}

// Test_ProcessCheck_ReportedValuesNormalizedOnIngest locks in that reported hashes and signer
// fingerprints are stored in the normalized form the controller stores them in, whatever case and
// separator style the client sent.
func Test_ProcessCheck_ReportedValuesNormalizedOnIngest(t *testing.T) {
	instance := newInstance()

	updated := instance.Apply(processListResponse(upperHash, []string{separatedPrint}), nil)

	data := instance.InstanceData

	require.True(t, updated)
	require.Len(t, data.ProcessList.Processes, 1)
	require.Equal(t, lowerHash, data.ProcessList.Processes[0].Hash)
	require.Equal(t, []string{lowerPrint}, data.ProcessList.Processes[0].SignerFingerprints)
}

// Test_ProcessCheck_NormalizationDoesNotMutateResponse locks in that ingest normalization leaves
// the caller's protobuf message untouched.
func Test_ProcessCheck_NormalizationDoesNotMutateResponse(t *testing.T) {
	instance := newInstance()
	response := processListResponse(upperHash, []string{separatedPrint})

	instance.Apply(response, nil)

	require.Equal(t, upperHash, response.GetProcessList().Processes[0].Hash)
	require.Equal(t, []string{separatedPrint}, response.GetProcessList().Processes[0].SignerFingerprints)
}

// Test_ProcessCheck_RepeatedResponseNotSeenAsChange locks in that re-reporting the same process in
// its uppercase form does not read as a posture change once the stored copy is normalized.
func Test_ProcessCheck_RepeatedResponseNotSeenAsChange(t *testing.T) {
	instance := newInstance()
	require.True(t, instance.Apply(processListResponse(upperHash, []string{separatedPrint}), nil))

	updated := instance.Apply(processListResponse(upperHash, []string{separatedPrint}), nil)

	require.False(t, updated)
}
