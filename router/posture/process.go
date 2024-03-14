package posture

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/db"
)

type ProcessCheck struct {
	*edge_ctrl_pb.DataState_PostureCheck
	*edge_ctrl_pb.DataState_PostureCheck_ProcessMulti
}

func (p *ProcessCheck) Evaluate(cache *Cache) *CheckError {
	switch p.Semantic {
	case db.SemanticAllOf:
		return p.requireAll(cache)
	case db.SemanticAnyOf:
		return p.requireOne(cache)
	default:
		pfxlog.Logger().Panicf("invalid semantic, expected %s or %s got [%s]", db.SemanticAllOf, db.SemanticAnyOf, p.Semantic)
		return nil
	}
}

func (p *ProcessCheck) requireAll(cache *Cache) *CheckError {
	if cache == nil {
		return &CheckError{
			Id:    p.Id,
			Name:  p.Name,
			Cause: NilStateError,
		}
	}
	cacheProcesses := map[string]*edge_client_pb.PostureResponse_Process{}

	for _, process := range cache.ProcessList.Processes {
		cacheProcesses[process.Path] = process
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

		failedValue := p.compareProcesses(cache.Os.Os.Type, cacheProcess, process)

		if failedValue != nil {
			allInListError.FailedValues = append(allInListError.FailedValues, *failedValue)
		}
	}

	if len(allInListError.FailedValues) > 0 {
		for _, process := range cache.ProcessList.Processes {
			allInListError.GivenValues = append(allInListError.GivenValues, &edge_ctrl_pb.DataState_PostureCheck_Process{
				OsType:       cache.Os.Os.Type,
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

func (p *ProcessCheck) requireOne(cache *Cache) *CheckError {
	if cache == nil {
		return &CheckError{
			Id:    p.Id,
			Name:  p.Name,
			Cause: NilStateError,
		}
	}
	cacheProcesses := map[string]*edge_client_pb.PostureResponse_Process{}

	for _, process := range cache.ProcessList.Processes {
		cacheProcesses[process.Path] = process
	}

	anyInList := &AnyInListError[*edge_ctrl_pb.DataState_PostureCheck_Process]{}

	for _, process := range p.Processes {
		cacheProcess := cacheProcesses[process.Path]

		failedValue := p.compareProcesses(cache.Os.Os.Type, cacheProcess, process)

		if failedValue != nil {
			anyInList.FailedValues = append(anyInList.FailedValues, *failedValue)
		} else {
			return nil
		}
	}

	for _, process := range cache.ProcessList.Processes {
		anyInList.GivenValues = append(anyInList.GivenValues, &edge_ctrl_pb.DataState_PostureCheck_Process{
			OsType:       cache.Os.Os.Type,
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

	if valid.Path != given.Path {
		result.Reason = fmt.Errorf("paths do not match, given %s, expected: %s", given.Path, valid.Path)
		return result
	}

	if valid.OsType != osType {
		result.Reason = fmt.Errorf("os types do not match, given %s, expected: %s", osType, valid.OsType)
		return result
	}

	if len(valid.Hashes) > 0 && !stringz.Contains(valid.Hashes, given.Hash) {
		result.Reason = fmt.Errorf("hash is not valid, given %s, expected one of: %v", given.Hash, valid.Hashes)
		return result
	}

	if len(valid.Fingerprints) > 0 {
		validPrints := map[string]struct{}{}

		for _, validPrint := range valid.Fingerprints {
			validPrints[validPrint] = struct{}{}
		}

		validPrintFound := false
		for _, givenPrint := range given.SignerFingerprints {
			if _, ok := validPrints[givenPrint]; ok {
				validPrintFound = true
				break
			}
		}

		if !validPrintFound {
			result.Reason = fmt.Errorf("valid signer not found, given: %v, expected one of: %v", given.SignerFingerprints, valid.Hashes)
			return result
		}
	}

	return nil
}
