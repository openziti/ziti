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

	"github.com/blang/semver"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/db"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

var _ PostureCheckSubType = &PostureCheckOperatingSystem{}

type PostureCheckOperatingSystem struct {
	OperatingSystems []OperatingSystem
}

func (p *PostureCheckOperatingSystem) TypeId() string {
	return db.PostureCheckTypeOs
}

func (p *PostureCheckOperatingSystem) fillProtobuf(msg *edge_cmd_pb.PostureCheck) {
	osList := &edge_cmd_pb.PostureCheck_OsList{}
	for _, os := range p.OperatingSystems {
		osList.OsList = append(osList.OsList, &edge_cmd_pb.PostureCheck_Os{
			OsType:     os.OsType,
			OsVersions: os.OsVersions,
		})
	}

	msg.Subtype = &edge_cmd_pb.PostureCheck_OsList_{
		OsList: osList,
	}
}

func (p *PostureCheckOperatingSystem) fillFromProtobuf(msg *edge_cmd_pb.PostureCheck) error {
	if osList_, ok := msg.Subtype.(*edge_cmd_pb.PostureCheck_OsList_); ok {
		if osList := osList_.OsList; osList != nil {
			for _, os := range osList.OsList {
				p.OperatingSystems = append(p.OperatingSystems, OperatingSystem{
					OsType:     os.OsType,
					OsVersions: os.OsVersions,
				})
			}
		}
	} else {
		return errors.Errorf("expected posture check sub type data of operation system, but got %T", msg.Subtype)
	}
	return nil
}

func (p *PostureCheckOperatingSystem) LastUpdatedAt(id string, pd *PostureData) *time.Time {
	return nil
}

func (p *PostureCheckOperatingSystem) GetTimeoutRemainingSeconds(_ string, _ *PostureData) int64 {
	return PostureCheckNoTimeout
}

func (p *PostureCheckOperatingSystem) GetTimeoutSeconds() int64 {
	return PostureCheckNoTimeout
}

func (p *PostureCheckOperatingSystem) FailureValues(_ string, pd *PostureData) PostureCheckFailureValues {
	return &PostureCheckFailureValuesOperatingSystem{
		ActualValue:   pd.Os,
		ExpectedValue: p.OperatingSystems,
	}
}

func (p *PostureCheckOperatingSystem) Evaluate(_ string, pd *PostureData) bool {
	if pd.Os.TimedOut {
		return false
	}

	validOses := getValidOses(p.OperatingSystems)

	osType := strings.ToLower(pd.Os.Type)

	if validOsVersions, isValidOs := validOses[osType]; isValidOs {
		if len(validOsVersions) == 0 {
			return true

		}

		dataVersion, err := semver.Make(pd.Os.Version)
		if err != nil {
			pfxlog.Logger().Errorf("could not parse versions %s: %v", pd.Os.Version, err)
			return false
		}

		for _, validOsVersion := range validOsVersions {
			if (*validOsVersion)(dataVersion) {
				return true
			}
		}
	}

	return false
}

func getValidOses(oses []OperatingSystem) map[string][]*semver.Range {
	validOses := map[string][]*semver.Range{}

	for _, os := range oses {
		osType := strings.ToLower(os.OsType)
		validOses[osType] = []*semver.Range{} //last os definition wins if redeclared

		for _, strVersion := range os.OsVersions {
			semVer, err := semver.ParseRange(strVersion)
			if err != nil {
				pfxlog.Logger().Errorf("could not parse version %s: %v", strVersion, err)
				continue
			}
			validOses[osType] = append(validOses[osType], &semVer)
		}
	}

	return validOses
}

type OperatingSystem struct {
	OsType     string
	OsVersions []string
}

func newPostureCheckOperatingSystem() PostureCheckSubType {
	return &PostureCheckOperatingSystem{}
}

func (p *PostureCheckOperatingSystem) fillFrom(_ Env, tx *bbolt.Tx, check *db.PostureCheck, subType db.PostureCheckSubType) error {
	subCheck := subType.(*db.PostureCheckOperatingSystem)

	if subCheck == nil {
		return fmt.Errorf("could not convert os check to bolt type")
	}

	for _, osMatch := range subCheck.OperatingSystems {
		p.OperatingSystems = append(p.OperatingSystems, OperatingSystem{
			OsType:     osMatch.OsType,
			OsVersions: osMatch.OsVersions,
		})
	}

	return nil
}

func (p *PostureCheckOperatingSystem) validateOsVersions() error {
	for _, os := range p.OperatingSystems {
		for versionIdx, version := range os.OsVersions {
			if _, err := semver.ParseRange(version); err != nil {
				msg := fmt.Sprintf("invalid version for os: [%s], version: [%s]: %v ", os, version, err)
				return errorz.NewFieldError(msg, fmt.Sprintf("operatingSystems[%s][%d]", os, versionIdx), msg)
			}
		}
	}

	return nil
}

func (p *PostureCheckOperatingSystem) toBoltEntityForCreate(*bbolt.Tx, Env) (db.PostureCheckSubType, error) {
	ret := &db.PostureCheckOperatingSystem{
		OperatingSystems: []db.OperatingSystem{},
	}

	if err := p.validateOsVersions(); err != nil {
		return nil, err
	}

	for _, osMatch := range p.OperatingSystems {
		ret.OperatingSystems = append(ret.OperatingSystems, db.OperatingSystem{
			OsType:     osMatch.OsType,
			OsVersions: osMatch.OsVersions,
		})
	}

	return ret, nil
}

type PostureCheckFailureValuesOperatingSystem struct {
	ActualValue   PostureResponseOs
	ExpectedValue []OperatingSystem
}

func (p PostureCheckFailureValuesOperatingSystem) Expected() interface{} {
	return p.ExpectedValue
}

func (p PostureCheckFailureValuesOperatingSystem) Actual() interface{} {
	return p.ActualValue
}
