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
	"github.com/blang/semver"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/foundation/util/errorz"
	"go.etcd.io/bbolt"
	"strings"
)

var _ PostureCheckSubType = &PostureCheckOperatingSystem{}

type PostureCheckOperatingSystem struct {
	OperatingSystems []OperatingSystem
}

func (p *PostureCheckOperatingSystem) GetTimeoutRemainingSeconds(_ string, _ *PostureData) int64 {
	return PostureCheckNoTimeout
}

func (p *PostureCheckOperatingSystem) GetTimeoutSeconds() int64 {
	return PostureCheckNoTimeout
}

func (p *PostureCheckOperatingSystem) FailureValues(_ string, pd *PostureData) PostureCheckFailureValues {
	return &PostureCheckFailureValuesOperatingSystem{
		ActualValue: pd.Os,
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

type version struct {
	value       int64
	orHigher    bool
	subVersions map[int64]*version
}

func (version *version) isValid(checkVersions []int64) bool {
	if len(checkVersions) == 0 {
		return false //not enough versions to check
	}

	if checkVersions[0] == version.value {
		if len(version.subVersions) == 0 {
			return true
		}

		for _, subVersion := range version.subVersions {
			return subVersion.isValid(checkVersions[1:])
		}
	} else if version.orHigher && checkVersions[0] > version.value {
		return true
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

func (p *PostureCheckOperatingSystem) fillFrom(handler Handler, tx *bbolt.Tx, check *persistence.PostureCheck, subType persistence.PostureCheckSubType) error {
	subCheck := subType.(*persistence.PostureCheckOperatingSystem)

	if subCheck == nil {
		return fmt.Errorf("could not covert os check to bolt type")
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

func (p *PostureCheckOperatingSystem) toBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	ret := &persistence.PostureCheckOperatingSystem{
		OperatingSystems: []persistence.OperatingSystem{},
	}

	if err := p.validateOsVersions(); err != nil {
		return nil, err
	}

	for _, osMatch := range p.OperatingSystems {
		ret.OperatingSystems = append(ret.OperatingSystems, persistence.OperatingSystem{
			OsType:     osMatch.OsType,
			OsVersions: osMatch.OsVersions,
		})
	}

	return ret, nil
}

func (p *PostureCheckOperatingSystem) toBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	ret := &persistence.PostureCheckOperatingSystem{
		OperatingSystems: []persistence.OperatingSystem{},
	}

	if err := p.validateOsVersions(); err != nil {
		return nil, err
	}

	for _, osMatch := range p.OperatingSystems {
		ret.OperatingSystems = append(ret.OperatingSystems, persistence.OperatingSystem{
			OsType:     osMatch.OsType,
			OsVersions: osMatch.OsVersions,
		})
	}

	return ret, nil
}

func (p *PostureCheckOperatingSystem) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	ret := &persistence.PostureCheckOperatingSystem{
		OperatingSystems: []persistence.OperatingSystem{},
	}

	if err := p.validateOsVersions(); err != nil {
		return nil, err
	}

	for _, osMatch := range p.OperatingSystems {
		ret.OperatingSystems = append(ret.OperatingSystems, persistence.OperatingSystem{
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
