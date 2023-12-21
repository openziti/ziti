/*
	Copyright NetFoundry Inc.

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
	"strings"
	"time"

	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/db"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

var _ PostureCheckSubType = &PostureCheckDomains{}

type PostureCheckDomains struct {
	Domains []string
}

func (p *PostureCheckDomains) TypeId() string {
	return db.PostureCheckTypeDomain
}

func (p *PostureCheckDomains) fillProtobuf(msg *edge_cmd_pb.PostureCheck) {
	msg.Subtype = &edge_cmd_pb.PostureCheck_Domains_{
		Domains: &edge_cmd_pb.PostureCheck_Domains{
			Domains: p.Domains,
		},
	}
}

func (p *PostureCheckDomains) fillFromProtobuf(msg *edge_cmd_pb.PostureCheck) error {
	if domains_, ok := msg.Subtype.(*edge_cmd_pb.PostureCheck_Domains_); ok {
		if domains := domains_.Domains; domains != nil {
			p.Domains = domains.Domains
		}
	} else {
		return errors.Errorf("expected posture check sub type data of mfa, but got %T", msg.Subtype)
	}
	return nil
}

func (p *PostureCheckDomains) LastUpdatedAt(string, *PostureData) *time.Time {
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

func (p *PostureCheckDomains) ActualValue(_ string, pd *PostureData) interface{} {
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
		if strings.EqualFold(domain, pd.Domain.Name) {
			return true
		}
	}

	return false
}

func newPostureCheckWindowsDomains() PostureCheckSubType {
	return &PostureCheckDomains{}
}

func (p *PostureCheckDomains) fillFrom(_ Env, _ *bbolt.Tx, _ *db.PostureCheck, subType db.PostureCheckSubType) error {
	subCheck := subType.(*db.PostureCheckWindowsDomains)

	if subCheck == nil {
		return fmt.Errorf("could not convert domain check to bolt type")
	}

	p.Domains = subCheck.Domains
	return nil
}

func (p *PostureCheckDomains) toBoltEntityForCreate(*bbolt.Tx, Env) (db.PostureCheckSubType, error) {
	return &db.PostureCheckWindowsDomains{
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
