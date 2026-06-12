package posture

import (
	"testing"

	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/db"
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

// A single PROCESS check with no signer requirement arrives with Fingerprints []string{""} (the
// controller packs the empty signer into a one-element slice). That empty entry is not a real
// constraint, so a running process reporting no signers must pass.
func TestProcessCheck_EmptyStringFingerprint_TreatedAsNoConstraint(t *testing.T) {
	for _, semantic := range []string{db.SemanticAnyOf, db.SemanticAllOf} {
		t.Run(semantic, func(t *testing.T) {
			check := newProcessCheck(semantic)
			check.Processes[0].Fingerprints = []string{""}

			data := &InstanceData{
				Os: &edge_client_pb.PostureResponse_Os{
					Os: &edge_client_pb.PostureResponse_OperatingSystem{Type: "Windows"},
				},
				ProcessList: &edge_client_pb.PostureResponse_ProcessList{
					Processes: []*edge_client_pb.PostureResponse_Process{
						{Path: testProcPath, IsRunning: true}, // running, reports no signer fingerprints
					},
				},
			}

			if result := check.Evaluate(data); result != nil {
				t.Fatalf("semantic %s: expected pass when the only signer fingerprint is empty, got: %v", semantic, result.Cause)
			}
		})
	}
}

// Same empty-signer case for a multi-process (PROCESS_MULTI) check: every configured process has
// an empty signer entry, and every reported process is running with no signers. Must pass under
// both semantics.
func TestProcessCheck_Multi_EmptyStringFingerprints_TreatedAsNoConstraint(t *testing.T) {
	const otherPath = "C:\\Windows\\System32\\calc.exe"

	for _, semantic := range []string{db.SemanticAnyOf, db.SemanticAllOf} {
		t.Run(semantic, func(t *testing.T) {
			check := &ProcessCheck{
				DataState_PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{Id: "test-id", Name: "my-proc-multi"},
				DataState_PostureCheck_ProcessMulti: &edge_ctrl_pb.DataState_PostureCheck_ProcessMulti{
					Semantic: semantic,
					Processes: []*edge_ctrl_pb.DataState_PostureCheck_Process{
						{OsType: "Windows", Path: testProcPath, Fingerprints: []string{""}},
						{OsType: "Windows", Path: otherPath, Fingerprints: []string{""}},
					},
				},
			}

			data := &InstanceData{
				Os: &edge_client_pb.PostureResponse_Os{
					Os: &edge_client_pb.PostureResponse_OperatingSystem{Type: "Windows"},
				},
				ProcessList: &edge_client_pb.PostureResponse_ProcessList{
					Processes: []*edge_client_pb.PostureResponse_Process{
						{Path: testProcPath, IsRunning: true},
						{Path: otherPath, IsRunning: true},
					},
				},
			}

			if result := check.Evaluate(data); result != nil {
				t.Fatalf("semantic %s: expected pass when signer fingerprints are empty, got: %v", semantic, result.Cause)
			}
		})
	}
}

// Same empty-constraint case for hashes: a check with no hash requirement arrives as
// Hashes []string{""}, which is not a real constraint, so a running process passes regardless of
// the hash it reports.
func TestProcessCheck_EmptyStringHash_TreatedAsNoConstraint(t *testing.T) {
	for _, semantic := range []string{db.SemanticAnyOf, db.SemanticAllOf} {
		t.Run(semantic, func(t *testing.T) {
			check := newProcessCheck(semantic)
			check.Processes[0].Hashes = []string{""}

			data := &InstanceData{
				Os: &edge_client_pb.PostureResponse_Os{
					Os: &edge_client_pb.PostureResponse_OperatingSystem{Type: "Windows"},
				},
				ProcessList: &edge_client_pb.PostureResponse_ProcessList{
					Processes: []*edge_client_pb.PostureResponse_Process{
						{Path: testProcPath, IsRunning: true, Hash: "feedface"},
					},
				},
			}

			if result := check.Evaluate(data); result != nil {
				t.Fatalf("semantic %s: expected pass when the only hash constraint is empty, got: %v", semantic, result.Cause)
			}
		})
	}
}

// With one process passing and one failing, the semantic decides the outcome: AnyOf passes
// (at least one good), AllOf fails (not all good).
func TestProcessCheck_MultiSemantics_OnePassOneFail(t *testing.T) {
	const failPath = "C:\\Windows\\System32\\calc.exe"

	newCheck := func(semantic string) *ProcessCheck {
		return &ProcessCheck{
			DataState_PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{Id: "test-id", Name: "my-proc-multi"},
			DataState_PostureCheck_ProcessMulti: &edge_ctrl_pb.DataState_PostureCheck_ProcessMulti{
				Semantic: semantic,
				Processes: []*edge_ctrl_pb.DataState_PostureCheck_Process{
					{OsType: "Windows", Path: testProcPath},                              // no constraint -> passes
					{OsType: "Windows", Path: failPath, Hashes: []string{"deadbeef"}},   // hash required
				},
			},
		}
	}

	// reported: both running, but the second reports a hash that does not match "deadbeef"
	data := &InstanceData{
		Os: &edge_client_pb.PostureResponse_Os{
			Os: &edge_client_pb.PostureResponse_OperatingSystem{Type: "Windows"},
		},
		ProcessList: &edge_client_pb.PostureResponse_ProcessList{
			Processes: []*edge_client_pb.PostureResponse_Process{
				{Path: testProcPath, IsRunning: true},
				{Path: failPath, IsRunning: true, Hash: "feedface"},
			},
		},
	}

	if result := newCheck(db.SemanticAnyOf).Evaluate(data); result != nil {
		t.Fatalf("AnyOf: expected pass (one process matches), got: %v", result.Cause)
	}
	if result := newCheck(db.SemanticAllOf).Evaluate(data); result == nil {
		t.Fatalf("AllOf: expected failure (one process does not match), got pass")
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
