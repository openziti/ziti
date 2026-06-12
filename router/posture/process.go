package posture

import (
	"errors"
	"fmt"
	"strings"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/db"
)

type ProcessCheck struct {
	*edge_ctrl_pb.DataState_PostureCheck
	*edge_ctrl_pb.DataState_PostureCheck_ProcessMulti
}

func (p *ProcessCheck) Evaluate(data *InstanceData) *CheckError {
	switch p.Semantic {
	case db.SemanticAllOf:
		return p.requireAll(data)
	case db.SemanticAnyOf:
		return p.requireOne(data)
	default:
		pfxlog.Logger().Panicf("invalid semantic, expected %s or %s got [%s]", db.SemanticAllOf, db.SemanticAnyOf, p.Semantic)
		return nil
	}
}

func (p *ProcessCheck) requireAll(data *InstanceData) *CheckError {
	if data == nil {
		return &CheckError{
			Id:    p.Id,
			Name:  p.Name,
			Cause: NilStateError,
		}
	}
	cacheProcesses := map[string]*edge_client_pb.PostureResponse_Process{}

	for _, process := range data.ProcessList.GetProcesses() {
		cacheProcesses[process.Path] = process
	}

	osType := ""

	if data.Os != nil {
		osType = data.Os.Os.GetType()
	}

	allInListError := &AllInListError[*edge_ctrl_pb.DataState_PostureCheck_Process]{}

	for _, process := range p.Processes {
		if process.Path == "" {
			return &CheckError{
				Id:    p.Id,
				Name:  p.Name,
				Cause: fmt.Errorf("invalid path in process check, '%s'", process.Path),
			}
		}

		cacheProcess, ok := cacheProcesses[process.Path]

		if !ok {
			return &CheckError{
				Id:    p.Id,
				Name:  p.Name,
				Cause: fmt.Errorf("the process path %s was not found, it hasn't been reported or isn't running on the client", process.Path),
			}
		}

		failedValue := p.compareProcesses(osType, cacheProcess, process)

		if failedValue != nil {
			allInListError.FailedValues = append(allInListError.FailedValues, *failedValue)
		}
	}

	if len(allInListError.FailedValues) > 0 {
		for _, process := range data.ProcessList.GetProcesses() {
			allInListError.GivenValues = append(allInListError.GivenValues, &edge_ctrl_pb.DataState_PostureCheck_Process{
				OsType:       osType,
				Path:         process.Path,
				Hashes:       []string{process.Hash},
				Fingerprints: process.SignerFingerprints,
			})
		}

		return &CheckError{
			Id:    p.Id,
			Name:  p.Name,
			Cause: allInListError,
		}
	}

	return nil
}

func (p *ProcessCheck) requireOne(data *InstanceData) *CheckError {
	if data == nil {
		return &CheckError{
			Id:    p.Id,
			Name:  p.Name,
			Cause: NilStateError,
		}
	}
	cacheProcesses := map[string]*edge_client_pb.PostureResponse_Process{}

	for _, process := range data.ProcessList.GetProcesses() {
		cacheProcesses[process.Path] = process
	}

	anyInList := &AnyInListError[*edge_ctrl_pb.DataState_PostureCheck_Process]{}

	osType := ""

	if data.Os != nil {
		osType = data.Os.Os.GetType()
	}

	for _, process := range p.Processes {
		cacheProcess := cacheProcesses[process.Path]

		failedValue := p.compareProcesses(osType, cacheProcess, process)

		if failedValue != nil {
			anyInList.FailedValues = append(anyInList.FailedValues, *failedValue)
		} else {
			return nil
		}
	}

	for _, process := range data.ProcessList.GetProcesses() {
		anyInList.GivenValues = append(anyInList.GivenValues, &edge_ctrl_pb.DataState_PostureCheck_Process{
			OsType:       osType,
			Path:         process.Path,
			Hashes:       []string{process.Hash},
			Fingerprints: process.SignerFingerprints,
		})
	}

	return &CheckError{
		Id:    p.Id,
		Name:  p.Name,
		Cause: anyInList,
	}
}

func (p *ProcessCheck) compareProcesses(osType string, given *edge_client_pb.PostureResponse_Process, valid *edge_ctrl_pb.DataState_PostureCheck_Process) *FailedValueError[*edge_ctrl_pb.DataState_PostureCheck_Process] {
	result := &FailedValueError[*edge_ctrl_pb.DataState_PostureCheck_Process]{
		ExpectedValue: valid,
		GivenValue:    nil,
		Reason:        nil,
	}

	if given == nil {
		result.Reason = fmt.Errorf("no matching process by path %s, the process hasn't been sumitted or isn't running", valid.Path)
		return result
	}

	if !given.IsRunning {
		result.Reason = errors.New("process is reported as not running")
		return result
	}

	if valid.Path != given.Path {
		result.Reason = fmt.Errorf("paths do not match, given %s, expected: %s", given.Path, valid.Path)
		return result
	}

	if !strings.EqualFold(strings.ToLower(valid.OsType), strings.ToLower(osType)) {
		result.Reason = fmt.Errorf("os types do not match, given %s, expected: %s", osType, valid.OsType)
		return result
	}

	// an empty hash is not a real constraint (a check with no hash arrives as []string{""}),
	// so only the non-empty entries count.
	validHashes := make([]string, 0, len(valid.Hashes))
	for _, h := range valid.Hashes {
		if h != "" {
			validHashes = append(validHashes, h)
		}
	}

	if len(validHashes) > 0 && !stringz.Contains(validHashes, given.Hash) {
		result.Reason = fmt.Errorf("hash is not valid, given %s, expected one of: %v", given.Hash, validHashes)
		return result
	}

	// an empty fingerprint is not a real constraint (a single PROCESS check with no signer
	// arrives as []string{""}), so only the non-empty entries count.
	validPrints := map[string]struct{}{}
	for _, validPrint := range valid.Fingerprints {
		if validPrint != "" {
			validPrints[validPrint] = struct{}{}
		}
	}

	if len(validPrints) > 0 {
		validPrintFound := false
		for _, givenPrint := range given.SignerFingerprints {
			if _, ok := validPrints[givenPrint]; ok {
				validPrintFound = true
				break
			}
		}

		if !validPrintFound {
			result.Reason = fmt.Errorf("valid signer not found, given: %v, expected one of: %v", given.SignerFingerprints, valid.Fingerprints)
			return result
		}
	}

	return nil
}
