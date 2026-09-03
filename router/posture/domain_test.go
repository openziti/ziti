package posture

import (
	"testing"

	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
)

func newDomainCheck(domains ...string) *DomainCheck {
	return &DomainCheck{
		DataState_PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{Id: "domain-check", Name: "domain-check"},
		DataState_PostureCheck_Domains: &edge_ctrl_pb.DataState_PostureCheck_Domains{
			Domains: domains,
		},
	}
}

// Test_DomainCheck_NoDomainReported locks in that a session which has not sent a domain posture
// response fails the check rather than dereferencing the nil Domain state.
func Test_DomainCheck_NoDomainReported(t *testing.T) {
	check := newDomainCheck("corp.example.com")

	err := check.Evaluate(&InstanceData{})

	require.NotNil(t, err, "no domain data reported: the check must fail")
	require.ErrorIs(t, err.Cause, NilStateError)
}

// Test_DomainCheck_NilState locks in the same for a wholly absent posture instance.
func Test_DomainCheck_NilState(t *testing.T) {
	check := newDomainCheck("corp.example.com")

	err := check.Evaluate(nil)

	require.NotNil(t, err, "no posture state at all: the check must fail")
	require.ErrorIs(t, err.Cause, NilStateError)
}

// Test_DomainCheck_MatchPasses keeps the guards from swallowing a legitimate pass.
func Test_DomainCheck_MatchPasses(t *testing.T) {
	check := newDomainCheck("other.example.com", "corp.example.com")
	state := &InstanceData{
		Domain: &edge_client_pb.PostureResponse_Domain{Name: "corp.example.com"},
	}

	require.Nil(t, check.Evaluate(state), "the reported domain is in the valid list")
}

// Test_DomainCheck_NoMatchFails covers the reported-but-not-matching case.
func Test_DomainCheck_NoMatchFails(t *testing.T) {
	check := newDomainCheck("corp.example.com")
	state := &InstanceData{
		Domain: &edge_client_pb.PostureResponse_Domain{Name: "other.example.com"},
	}

	err := check.Evaluate(state)

	require.NotNil(t, err, "the reported domain is not in the valid list")
	require.NotErrorIs(t, err.Cause, NilStateError)
}
