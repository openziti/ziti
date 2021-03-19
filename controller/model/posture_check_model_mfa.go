/*
	Copyright NetFoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package model

import (
	"fmt"
	"github.com/openziti/edge/controller/persistence"
	"go.etcd.io/bbolt"
)

var _ PostureCheckSubType = &PostureCheckMfa{}

type PostureCheckMfa struct {
}

func (p *PostureCheckMfa) FailureValues(apiSessionId string, pd *PostureData) PostureCheckFailureValues {
	ret := &PostureCheckFailureValuesMfa{
		ActualValue:   false,
		ExpectedValue: true,
	}

	if val, ok := pd.ApiSessions[apiSessionId]; ok {
		ret.ActualValue = val.Mfa.PassedMfa
	}

	return ret
}

func (p *PostureCheckMfa) Evaluate(apiSessionId string, pd *PostureData) bool {
	apiSessionData := pd.ApiSessions[apiSessionId]

	if apiSessionData != nil {
		return apiSessionData.Mfa.PassedMfa
	}

	return false
}

func newPostureCheckMfa() PostureCheckSubType {
	return &PostureCheckMfa{}
}

func (p *PostureCheckMfa) fillFrom(handler Handler, tx *bbolt.Tx, check *persistence.PostureCheck, subType persistence.PostureCheckSubType) error {
	subCheck := subType.(*persistence.PostureCheckMfa)

	if subCheck == nil {
		return fmt.Errorf("could not covert domain check to bolt type")
	}

	return nil
}

func (p *PostureCheckMfa) toBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckMfa{}, nil
}

func (p *PostureCheckMfa) toBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckMfa{}, nil
}

func (p *PostureCheckMfa) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckMfa{}, nil
}

type PostureCheckFailureValuesMfa struct {
	ActualValue   bool
	ExpectedValue bool
}

func (p PostureCheckFailureValuesMfa) Expected() interface{} {
	return p.ExpectedValue
}

func (p PostureCheckFailureValuesMfa) Actual() interface{} {
	return p.ActualValue
}
