package posture

import (
	"testing"

	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/db"
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

// Regression for the discourse #5881 panic (router crash on dial). The router evaluated a process posture
// check while the client had reported a running process but NO OS posture data, so Cache.Os was nil.
// The failure-report loop dereferenced cache.Os.Os.Type and nil-panicked. The check must instead fail
// cleanly. The process is reported at the SAME path the check requires, so evaluation reaches the
// OS-mismatch / report-building code rather than short-circuiting on a missing process.
func TestProcessCheck_ProcessReportedButNilOs_DoesNotPanic(t *testing.T) {
	for _, semantic := range []string{db.SemanticAnyOf, db.SemanticAllOf} {
		t.Run(semantic, func(t *testing.T) {
			check := newProcessCheck(semantic)

			cache := &Cache{
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

			result := check.Evaluate(cache)

			if result == nil {
				t.Fatalf("expected a non-nil CheckError when OS posture data is missing, got nil")
			}
			if result.Name != "my-proc-multi" {
				t.Fatalf("expected CheckError for check 'my-proc-multi', got %q", result.Name)
			}
		})
	}
}

// Empty posture state (neither process list nor OS reported) must also fail cleanly, exercising the
// nil ProcessList / nil Os guards at the top of the evaluators.
func TestProcessCheck_NoPostureData_DoesNotPanic(t *testing.T) {
	for _, semantic := range []string{db.SemanticAnyOf, db.SemanticAllOf} {
		t.Run(semantic, func(t *testing.T) {
			check := newProcessCheck(semantic)

			result := check.Evaluate(&Cache{})

			if result == nil {
				t.Fatalf("expected a non-nil CheckError when no posture data has been reported, got nil")
			}
		})
	}
}

// A nil Cache (no posture state at all) must fail cleanly with NilStateError.
func TestProcessCheck_NilCache(t *testing.T) {
	for _, semantic := range []string{db.SemanticAnyOf, db.SemanticAllOf} {
		check := newProcessCheck(semantic)

		result := check.Evaluate(nil)

		if result == nil {
			t.Fatalf("semantic %s: expected a non-nil CheckError for nil Cache", semantic)
		}
		if result.Cause != NilStateError {
			t.Fatalf("semantic %s: expected NilStateError, got %v", semantic, result.Cause)
		}
	}
}
