/*
	Copyright NetFoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package model

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"go.etcd.io/bbolt"
	"time"
)

var _ PostureCheckSubType = &PostureCheckProcessMulti{}

type PostureCheckProcessMulti struct {
	PostureCheckId string
	Semantic       string
	Processes      []*ProcessMulti
}

func (p *PostureCheckProcessMulti) LastUpdatedAt(id string, pd *PostureData) *time.Time {
	return nil
}

func (p *PostureCheckProcessMulti) GetTimeoutSeconds() int64 {
	return PostureCheckNoTimeout
}

func (p *PostureCheckProcessMulti) GetTimeoutRemainingSeconds(_ string, _ *PostureData) int64 {
	return PostureCheckNoTimeout
}

type ProcessMulti struct {
	OsType             string
	Path               string
	Hashes             []string
	SignerFingerprints []string
}

func (p *PostureCheckProcessMulti) FailureValues(_ string, pd *PostureData) PostureCheckFailureValues {
	ret := &PostureCheckFailureValuesProcessMulti{
		ActualValue:   []PostureResponseProcess{},
		ExpectedValue: *p,
	}
	for _, processData := range pd.Processes {
		if processData.PostureCheckId == p.PostureCheckId {
			ret.ActualValue = []PostureResponseProcess{*processData}
		}
		break
	}

	return ret
}

func (p *PostureCheckProcessMulti) Evaluate(_ string, pd *PostureData) bool {
	return p.evaluate(pd) == nil
}

func (p *PostureCheckProcessMulti) evaluate(pd *PostureData) PostureCheckFailureValues {
	switch p.Semantic {
	case persistence.SemanticAllOf:
		return p.requireAll(pd)
	case persistence.SemanticAnyOf:
		return p.requireOne(pd)
	default:
		pfxlog.Logger().Panicf("invalid semantic, expected %s or %s got [%s]", persistence.SemanticAllOf, persistence.SemanticAnyOf, p.Semantic)
		return nil
	}
}

func (p *PostureCheckProcessMulti) requireAll(pd *PostureData) PostureCheckFailureValues {
	for _, process := range p.Processes {
		if process.Path == "" {
			pfxlog.Logger().Debug("invalid process path detected during posture check process multi AllOf evaluation")
			return &PostureCheckFailureValuesProcessMulti{
				ActualValue:   []PostureResponseProcess{},
				ExpectedValue: *p,
			}
		}

		if processData, ok := pd.ProcessPathMap[process.Path]; ok {
			if !processData.VerifyMultiCriteria(process) {
				return &PostureCheckFailureValuesProcessMulti{
					ActualValue:   []PostureResponseProcess{*processData},
					ExpectedValue: *p,
				}
			}
		} else {
			return &PostureCheckFailureValuesProcessMulti{
				ActualValue:   []PostureResponseProcess{},
				ExpectedValue: *p,
			}
		}
	}

	return nil
}

func (p *PostureCheckProcessMulti) requireOne(pd *PostureData) PostureCheckFailureValues {
	var actualValues []PostureResponseProcess

	for _, process := range p.Processes {
		if process.Path == "" {
			pfxlog.Logger().Debug("invalid process path detected during posture check process multi AnyOf evaluation")
			continue //ok to skip, only need 1 valid
		}

		if processData, ok := pd.ProcessPathMap[process.Path]; ok {
			if processData.VerifyMultiCriteria(process) {
				return nil //found 1
			} else {
				actualValues = append(actualValues, *processData)
			}
		}
	}

	return &PostureCheckFailureValuesProcessMulti{
		ActualValue:   actualValues,
		ExpectedValue: *p,
	}
}

func newPostureCheckProcessMulti() PostureCheckSubType {
	return &PostureCheckProcessMulti{}
}

func (p *PostureCheckProcessMulti) fillFrom(_ Handler, _ *bbolt.Tx, check *persistence.PostureCheck, subType persistence.PostureCheckSubType) error {
	subCheck := subType.(*persistence.PostureCheckProcessMulti)

	if subCheck == nil {
		return fmt.Errorf("could not covert process check process multi to bolt type")
	}

	p.PostureCheckId = check.Id
	p.Semantic = subCheck.Semantic

	for _, process := range subCheck.Processes {
		newProc := &ProcessMulti{
			OsType:             process.OsType,
			Path:               process.Path,
			Hashes:             process.Hashes,
			SignerFingerprints: process.SignerFingerprints,
		}

		p.Processes = append(p.Processes, newProc)
	}

	return nil
}

func (p *PostureCheckProcessMulti) toBoltEntityForCreate(_ *bbolt.Tx, _ Handler) (persistence.PostureCheckSubType, error) {
	ret := &persistence.PostureCheckProcessMulti{
		Semantic: p.Semantic,
	}

	for _, process := range p.Processes {
		newProc := &persistence.ProcessMulti{
			OsType:             process.OsType,
			Path:               process.Path,
			Hashes:             process.Hashes,
			SignerFingerprints: process.SignerFingerprints,
		}

		ret.Processes = append(ret.Processes, newProc)
	}

	return ret, nil
}

func (p *PostureCheckProcessMulti) toBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return p.toBoltEntityForCreate(tx, handler)
}

func (p *PostureCheckProcessMulti) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return p.toBoltEntityForCreate(tx, handler)
}

type PostureCheckFailureValuesProcessMulti struct {
	ActualValue   []PostureResponseProcess
	ExpectedValue PostureCheckProcessMulti
}

func (p PostureCheckFailureValuesProcessMulti) Expected() interface{} {
	return p.ExpectedValue
}

func (p PostureCheckFailureValuesProcessMulti) Actual() interface{} {
	return p.ActualValue
}
