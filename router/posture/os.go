package posture

import (
	"strings"

	"github.com/blang/semver"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
)

type OsCheck struct {
	*edge_ctrl_pb.DataState_PostureCheck
	*edge_ctrl_pb.DataState_PostureCheck_OsList
}

func (m *OsCheck) Evaluate(data *InstanceData) *CheckError {
	if data == nil || data.Os == nil {
		return &CheckError{
			Id:    m.Id,
			Name:  m.Name,
			Cause: NilStateError,
		}
	}

	osTypeFailure := &OneInListError[Str]{
		GivenValues: []Str{Str(data.Os.Os.GetType())},
	}

	var foundOs *edge_ctrl_pb.DataState_PostureCheck_Os = nil
	for _, os := range m.OsList {
		if !strings.EqualFold(strings.ToLower(os.OsType), strings.ToLower(data.Os.Os.GetType())) {
			osTypeFailure.ValidValues = append(osTypeFailure.ValidValues, Str(os.OsType))
		} else {
			foundOs = os
			break
		}
	}

	if foundOs == nil {
		return &CheckError{
			Id:    m.Id,
			Name:  m.Name,
			Cause: osTypeFailure,
		}
	}

	// type matched, no version constraint: any version passes (matches the controller's check).
	if len(foundOs.OsVersions) == 0 {
		return nil
	}

	dataVer, err := semver.Make(data.Os.Os.GetVersion())

	if err != nil {
		return &CheckError{
			Id:    m.Id,
			Name:  m.Name,
			Cause: err,
		}
	}

	osVersionFailure := &OneInListError[Str]{
		GivenValues: []Str{Str(data.Os.Os.GetVersion())},
	}

	for _, version := range foundOs.OsVersions {
		versionRange, err := semver.ParseRange(version)

		if err != nil {
			return &CheckError{
				Id:    m.Id,
				Name:  m.Name,
				Cause: err,
			}
		}

		if versionRange(dataVer) {
			return nil
		} else {
			osVersionFailure.ValidValues = append(osVersionFailure.ValidValues, Str(version))
		}
	}

	return &CheckError{
		Id:    m.Id,
		Name:  m.Name,
		Cause: osVersionFailure,
	}

}
