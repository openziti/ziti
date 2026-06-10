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

// Process reported at the required path but no OS posture: OS state is nil. Must fail cleanly, not panic.
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

// No posture data at all: nil ProcessList and nil OS state. Must fail cleanly, not panic.
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

// Nil Cache must fail with NilStateError, not panic.
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
