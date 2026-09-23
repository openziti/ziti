package handler_ctrl

import (
	"testing"

	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/stretchr/testify/require"
)

func Test_listenersEqual(t *testing.T) {
	listener := func(address string) *ctrl_pb.Listener {
		return &ctrl_pb.Listener{Address: address, Protocol: "tls", Groups: []string{"default"}, LocalBinding: "default"}
	}

	req := require.New(t)
	req.True(listenersEqual(nil, nil))
	req.True(listenersEqual(nil, []*ctrl_pb.Listener{}))
	req.True(listenersEqual([]*ctrl_pb.Listener{listener("tls:a:6000")}, []*ctrl_pb.Listener{listener("tls:a:6000")}))
	req.False(listenersEqual([]*ctrl_pb.Listener{listener("tls:a:6000")}, nil))
	req.False(listenersEqual([]*ctrl_pb.Listener{listener("tls:a:6000")}, []*ctrl_pb.Listener{listener("tls:b:6000")}))
	req.False(listenersEqual(
		[]*ctrl_pb.Listener{listener("tls:a:6000"), listener("tls:b:6000")},
		[]*ctrl_pb.Listener{listener("tls:b:6000"), listener("tls:a:6000")}))
}
