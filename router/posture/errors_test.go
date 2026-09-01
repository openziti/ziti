package posture

import (
	"testing"

	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
)

// Test_EvaluatePostureCheck_UnsupportedSubtype locks in that a check whose subtype the router does
// not recognize fails the check rather than calling the nil Checker CtrlCheckToLogic returns for it.
func Test_EvaluatePostureCheck_UnsupportedSubtype(t *testing.T) {
	postureCheck := &edge_ctrl_pb.DataState_PostureCheck{Id: "unknown-check", Name: "unknown-check"}

	err := EvaluatePostureCheck(postureCheck, &InstanceData{})

	require.NotNil(t, err, "an unrecognized subtype must fail the check")
	require.ErrorIs(t, err.Cause, UnsupportedCheckTypeError)
}

// Test_EvaluatePostureCheck_NoMacsReported drives the MAC nil-data case through the evaluation
// funnel both dial evaluation and posture push use, so a missing response never reaches the
// panic recovery.
func Test_EvaluatePostureCheck_NoMacsReported(t *testing.T) {
	postureCheck := &edge_ctrl_pb.DataState_PostureCheck{
		Id:   "mac-check",
		Name: "mac-check",
		Subtype: &edge_ctrl_pb.DataState_PostureCheck_Mac_{
			Mac: &edge_ctrl_pb.DataState_PostureCheck_Mac{MacAddresses: []string{"00:11:22:33:44:55"}},
		},
	}

	err := EvaluatePostureCheck(postureCheck, &InstanceData{})

	require.NotNil(t, err, "no MAC data reported: the check must fail")
	require.ErrorIs(t, err.Cause, NilStateError)
}

// Test_EvaluatePostureCheck_NoDomainReported does the same for the domain check.
func Test_EvaluatePostureCheck_NoDomainReported(t *testing.T) {
	postureCheck := &edge_ctrl_pb.DataState_PostureCheck{
		Id:   "domain-check",
		Name: "domain-check",
		Subtype: &edge_ctrl_pb.DataState_PostureCheck_Domains_{
			Domains: &edge_ctrl_pb.DataState_PostureCheck_Domains{Domains: []string{"corp.example.com"}},
		},
	}

	err := EvaluatePostureCheck(postureCheck, &InstanceData{})

	require.NotNil(t, err, "no domain data reported: the check must fail")
	require.ErrorIs(t, err.Cause, NilStateError)
}
