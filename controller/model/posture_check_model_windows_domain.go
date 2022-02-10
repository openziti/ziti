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
	"strings"
	"time"
)

var _ PostureCheckSubType = &PostureCheckDomains{}

type PostureCheckDomains struct {
	Domains []string
}

func (p *PostureCheckDomains) LastUpdatedAt(id string, pd *PostureData) *time.Time {
	return nil
}

func (p *PostureCheckDomains) GetTimeoutSeconds() int64 {
	return PostureCheckNoTimeout
}

func (p *PostureCheckDomains) GetTimeoutRemainingSeconds(_ string, _ *PostureData) int64 {
	return PostureCheckNoTimeout
}

func (p *PostureCheckDomains) FailureValues(_ string, pd *PostureData) PostureCheckFailureValues {
	return &PostureCheckFailureValuesDomain{
		ActualValue:   pd.Domain.Name,
		ExpectedValue: p.Domains,
	}
}

func (p *PostureCheckDomains) ActualValue(apiSessionId string, pd *PostureData) interface{} {
	return pd.Domain.Name
}

func (p *PostureCheckDomains) ExpectedValue() interface{} {
	return map[string]interface{}{
		"domains": p.Domains,
	}
}

func (p *PostureCheckDomains) Evaluate(_ string, pd *PostureData) bool {
	if pd.Domain.TimedOut {
		return false
	}

	for _, domain := range p.Domains {
		if strings.ToLower(domain) == strings.ToLower(pd.Domain.Name) {
			return true
		}
	}

	return false
}

func newPostureCheckWindowsDomains() PostureCheckSubType {
	return &PostureCheckDomains{}
}

func (p *PostureCheckDomains) fillFrom(handler Handler, tx *bbolt.Tx, check *persistence.PostureCheck, subType persistence.PostureCheckSubType) error {
	subCheck := subType.(*persistence.PostureCheckWindowsDomains)

	if subCheck == nil {
		return fmt.Errorf("could not covert domain check to bolt type")
	}

	p.Domains = subCheck.Domains
	return nil
}

func (p *PostureCheckDomains) toBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckWindowsDomains{
		Domains: p.Domains,
	}, nil
}

func (p *PostureCheckDomains) toBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckWindowsDomains{
		Domains: p.Domains,
	}, nil
}

func (p *PostureCheckDomains) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckWindowsDomains{
		Domains: p.Domains,
	}, nil
}

type PostureCheckFailureValuesDomain struct {
	ActualValue   string
	ExpectedValue []string
}

func (p PostureCheckFailureValuesDomain) Expected() interface{} {
	return p.ExpectedValue
}

func (p PostureCheckFailureValuesDomain) Actual() interface{} {
	return p.ActualValue
}
