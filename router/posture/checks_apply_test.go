package posture

import (
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/openziti/sdk-golang/v2/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common"
	"github.com/stretchr/testify/require"
)

type stubTotpParser struct {
	claims *common.TotpClaims
}

func (s *stubTotpParser) ParseTotpToken(string) (*common.TotpClaims, error) {
	return s.claims, nil
}

func processListResponse(procs ...*edge_client_pb.PostureResponse_Process) *edge_client_pb.PostureResponse {
	return &edge_client_pb.PostureResponse{
		Type: &edge_client_pb.PostureResponse_ProcessList_{
			ProcessList: &edge_client_pb.PostureResponse_ProcessList{Processes: procs},
		},
	}
}

func totpResponse(token string) *edge_client_pb.PostureResponse {
	return &edge_client_pb.PostureResponse{
		Type: &edge_client_pb.PostureResponse_TotpToken_{
			TotpToken: &edge_client_pb.PostureResponse_TotpToken{Token: token},
		},
	}
}

func Test_Apply_TotpResponsePreservesProcessListAndAdvancesMfa(t *testing.T) {
	req := require.New(t)

	claims := &common.TotpClaims{}
	claims.ApiSessionId = "api-session-1"
	claims.IssuedAt = jwt.NewNumericDate(time.Now())

	cache := NewCache(&stubTotpParser{claims: claims})

	cache.AddResponses("identity-1", "api-session-1", &edge_client_pb.PostureResponses{
		Responses: []*edge_client_pb.PostureResponse{
			processListResponse(&edge_client_pb.PostureResponse_Process{
				Path: "/usr/bin/foo", IsRunning: true, Hash: "abc",
			}),
		},
	})

	cache.AddResponses("identity-1", "api-session-1", &edge_client_pb.PostureResponses{
		Responses: []*edge_client_pb.PostureResponse{totpResponse("some-token")},
	})

	data := cache.GetInstance("api-session-1").Snapshot()
	req.NotNil(data.PassedMfaAt, "TOTP token should advance PassedMfaAt")
	req.Len(data.ProcessList.GetProcesses(), 1, "process list should survive a TOTP response")
}

func Test_Apply_PerPathProcessResponsesAccumulate(t *testing.T) {
	req := require.New(t)
	instance := newInstance()

	// the Go SDK reports each watched process as its own single-entry process list
	req.True(instance.Apply(processListResponse(
		&edge_client_pb.PostureResponse_Process{Path: "/usr/bin/foo", IsRunning: true}), nil))
	req.True(instance.Apply(processListResponse(
		&edge_client_pb.PostureResponse_Process{Path: "/usr/bin/bar", IsRunning: true}), nil))

	procs := instance.Snapshot().ProcessList.GetProcesses()
	req.Len(procs, 2, "both watched processes should be cached")
}

func Test_Apply_ProcessUpdateReplacesEntryByPath(t *testing.T) {
	req := require.New(t)
	instance := newInstance()

	req.True(instance.Apply(processListResponse(
		&edge_client_pb.PostureResponse_Process{Path: "/usr/bin/foo", IsRunning: true, Hash: "v1"},
		&edge_client_pb.PostureResponse_Process{Path: "/usr/bin/bar", IsRunning: true, Hash: "v1"}), nil))

	// identical re-send is not a change
	req.False(instance.Apply(processListResponse(
		&edge_client_pb.PostureResponse_Process{Path: "/usr/bin/foo", IsRunning: true, Hash: "v1"}), nil))

	// changed entry updates in place without disturbing the other path
	req.True(instance.Apply(processListResponse(
		&edge_client_pb.PostureResponse_Process{Path: "/usr/bin/foo", IsRunning: false, Hash: "v1"}), nil))

	procs := instance.Snapshot().ProcessList.GetProcesses()
	req.Len(procs, 2)
	byPath := map[string]*edge_client_pb.PostureResponse_Process{}
	for _, p := range procs {
		byPath[p.Path] = p
	}
	req.False(byPath["/usr/bin/foo"].IsRunning)
	req.True(byPath["/usr/bin/bar"].IsRunning)
}

func Test_Apply_EmptyResponseChangesNothing(t *testing.T) {
	req := require.New(t)
	instance := newInstance()

	req.True(instance.Apply(processListResponse(
		&edge_client_pb.PostureResponse_Process{Path: "/usr/bin/foo", IsRunning: true}), nil))

	req.False(instance.Apply(&edge_client_pb.PostureResponse{}, nil))
	req.Len(instance.Snapshot().ProcessList.GetProcesses(), 1)
}

func Test_Apply_OsUpdateDoesNotMutateSnapshots(t *testing.T) {
	req := require.New(t)
	instance := newInstance()

	osResponse := func(version string) *edge_client_pb.PostureResponse {
		return &edge_client_pb.PostureResponse{
			Type: &edge_client_pb.PostureResponse_Os{
				Os: &edge_client_pb.PostureResponse_OperatingSystem{Type: "linux", Version: version},
			},
		}
	}

	req.True(instance.Apply(osResponse("1.0.0"), nil))
	before := instance.Snapshot()

	req.True(instance.Apply(osResponse("2.0.0"), nil))

	req.Equal("1.0.0", before.Os.Os.GetVersion(), "prior snapshot must not observe later updates")
	req.Equal("2.0.0", instance.Snapshot().Os.Os.GetVersion())
}
