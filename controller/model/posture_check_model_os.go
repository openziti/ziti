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
)

type PostureCheckOperatingSystem struct {
	OperatingSystems []OperatingSystem
}

func (p *PostureCheckOperatingSystem) Evaluate(pd *PostureData) bool {
	if pd.Os.TimedOut {
		return false
	}

	validOses := map[string]map[string]struct{}{}

	for _, os := range p.OperatingSystems {
		osType := strings.ToLower(os.OsType)
		validOses[osType] = map[string]struct{}{}
		for _, version := range os.OsVersions {
			validOses[osType][strings.ToLower(version)] = struct{}{}
		}
	}

	osType := strings.ToLower(pd.Os.Type)
	osVersion := strings.ToLower(pd.Os.Version)
	if validOs, isValidOs := validOses[osType]; isValidOs {
		if len(validOs) == 0 {
			//any version
			return true
		}

		if _, isValidVersion := validOs[osVersion]; isValidVersion {
			return true
		}
	}

	return false
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

func (p *PostureCheckOperatingSystem) toBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	ret := &persistence.PostureCheckOperatingSystem{
		OperatingSystems: []persistence.OperatingSystem{},
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

	for _, osMatch := range p.OperatingSystems {
		ret.OperatingSystems = append(ret.OperatingSystems, persistence.OperatingSystem{
			OsType:     osMatch.OsType,
			OsVersions: osMatch.OsVersions,
		})
	}

	return ret, nil
}
