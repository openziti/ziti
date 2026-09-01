package posture

import (
	"testing"

	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
)

func newMacCheck(addresses ...string) *MacCheck {
	return &MacCheck{
		DataState_PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{Id: "mac-check", Name: "mac-check"},
		DataState_PostureCheck_Mac: &edge_ctrl_pb.DataState_PostureCheck_Mac{
			MacAddresses: addresses,
		},
	}
}

func macResponse(addresses ...string) *edge_client_pb.PostureResponse {
	return &edge_client_pb.PostureResponse{
		Type: &edge_client_pb.PostureResponse_Macs_{
			Macs: &edge_client_pb.PostureResponse_Macs{Addresses: addresses},
		},
	}
}

// Test_MacCheck_SeparatedAddressesNormalizedOnIngest locks in that reported addresses in the
// separated, mixed-case form ziti-sdk-c sends match a check whose values are stored normalized.
func Test_MacCheck_SeparatedAddressesNormalizedOnIngest(t *testing.T) {
	tests := []struct {
		name     string
		reported string
	}{
		{name: "colon separated", reported: "00:11:22:AA:BB:CC"},
		{name: "hyphen separated", reported: "00-11-22-aa-bb-cc"},
		{name: "dot separated", reported: "0011.22AA.bbcc"},
		{name: "uppercase unseparated", reported: "001122AABBCC"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := newInstance()
			check := newMacCheck("001122aabbcc")

			updated := instance.Apply(macResponse(test.reported), nil)

			data := instance.InstanceData

			require.True(t, updated)
			require.Nil(t, check.Evaluate(&data))
		})
	}
}

// Test_MacCheck_NormalizationDoesNotMutateResponse locks in that ingest normalization leaves the
// caller's protobuf message untouched.
func Test_MacCheck_NormalizationDoesNotMutateResponse(t *testing.T) {
	instance := newInstance()
	response := macResponse("00:11:22:AA:BB:CC")

	instance.Apply(response, nil)

	require.Equal(t, []string{"00:11:22:AA:BB:CC"}, response.GetMacs().Addresses)
}

// Test_MacCheck_RepeatedResponseNotSeenAsChange locks in that re-reporting the same addresses in
// their separated form does not read as a posture change once the stored copy is normalized.
func Test_MacCheck_RepeatedResponseNotSeenAsChange(t *testing.T) {
	instance := newInstance()
	require.True(t, instance.Apply(macResponse("00:11:22:AA:BB:CC"), nil))

	updated := instance.Apply(macResponse("00:11:22:AA:BB:CC"), nil)

	require.False(t, updated)
}

// Test_MacCheck_NoMacsReported locks in that a session which has not sent a MAC posture response
// fails the check rather than dereferencing the nil Macs state.
func Test_MacCheck_NoMacsReported(t *testing.T) {
	check := newMacCheck("00:11:22:33:44:55")

	err := check.Evaluate(&InstanceData{})

	require.NotNil(t, err, "no MAC data reported: the check must fail")
	require.ErrorIs(t, err.Cause, NilStateError)
}

// Test_MacCheck_NilState locks in the same for a wholly absent posture instance.
func Test_MacCheck_NilState(t *testing.T) {
	check := newMacCheck("00:11:22:33:44:55")

	err := check.Evaluate(nil)

	require.NotNil(t, err, "no posture state at all: the check must fail")
	require.ErrorIs(t, err.Cause, NilStateError)
}

// Test_MacCheck_MatchPasses keeps the guards from swallowing a legitimate pass.
func Test_MacCheck_MatchPasses(t *testing.T) {
	check := newMacCheck("00:11:22:33:44:55", "aa:bb:cc:dd:ee:ff")
	state := &InstanceData{
		Macs: &edge_client_pb.PostureResponse_Macs{Addresses: []string{"aa:bb:cc:dd:ee:ff"}},
	}

	require.Nil(t, check.Evaluate(state), "a reported address is in the valid list")
}

// Test_MacCheck_NoMatchFails covers the reported-but-not-matching case.
func Test_MacCheck_NoMatchFails(t *testing.T) {
	check := newMacCheck("00:11:22:33:44:55")
	state := &InstanceData{
		Macs: &edge_client_pb.PostureResponse_Macs{Addresses: []string{"aa:bb:cc:dd:ee:ff"}},
	}

	err := check.Evaluate(state)

	require.NotNil(t, err, "the reported address is not in the valid list")
	require.NotErrorIs(t, err.Cause, NilStateError)
}
