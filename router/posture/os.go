package posture

import "github.com/openziti/ziti/common/pb/edge_ctrl_pb"

type OsCheck struct {
	*edge_ctrl_pb.DataState_PostureCheck
	*edge_ctrl_pb.DataState_PostureCheck_OsList
}

func (m *OsCheck) Evaluate(state *Cache) *CheckError {
	return nil
}
