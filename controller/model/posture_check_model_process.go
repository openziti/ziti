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

var _ PostureCheckSubType = &PostureCheckProcess{}

type PostureCheckProcess struct {
	PostureCheckId string
	OsType         string
	Path           string
	Hashes         []string
	Fingerprint    string
}

func (p *PostureCheckProcess) GetTimeoutSeconds() int64 {
	return PostureCheckNoTimeout
}

func (p *PostureCheckProcess) GetTimeoutRemainingSeconds(_ string, _ *PostureData) int64 {
	return PostureCheckNoTimeout
}

func (p *PostureCheckProcess) FailureValues(_ string, pd *PostureData) PostureCheckFailureValues {
	ret := &PostureCheckFailureValuesProcess{
		ActualValue: PostureResponseProcess{
			PostureResponse:    nil,
			IsRunning:          false,
			BinaryHash:         "",
			SignerFingerprints: nil,
		},
		ExpectedValue: *p,
	}
	for _, processData := range pd.Processes {
		if processData.PostureCheckId == p.PostureCheckId {
			ret.ActualValue = *processData
		}
		break
	}

	return ret
}

func (p *PostureCheckProcess) Evaluate(_ string, pd *PostureData) bool {
	for _, process := range pd.Processes {
		if process.PostureCheckId == p.PostureCheckId {
			if process.TimedOut {
				return false
			}

			if !process.IsRunning {
				return false
			}

			if p.Fingerprint != "" {
				isFingerprintValid := false

				for _, fingerprint := range process.SignerFingerprints {
					if strings.EqualFold(p.Fingerprint, fingerprint) {
						isFingerprintValid = true
						break
					}
				}

				if !isFingerprintValid {
					return false
				}
			}

			if len(p.Hashes) > 0 {
				for _, hash := range p.Hashes {
					if strings.EqualFold(hash, process.BinaryHash) {
						return true
					}
				}
			} else {
				return true
			}
		}
	}

	return false
}

func newPostureCheckProcess() PostureCheckSubType {
	return &PostureCheckProcess{}
}

func (p *PostureCheckProcess) fillFrom(handler Handler, tx *bbolt.Tx, check *persistence.PostureCheck, subType persistence.PostureCheckSubType) error {
	subCheck := subType.(*persistence.PostureCheckProcess)

	if subCheck == nil {
		return fmt.Errorf("could not covert process check to bolt type")
	}

	p.PostureCheckId = check.Id
	p.OsType = subCheck.OperatingSystem
	p.Path = subCheck.Path
	p.Hashes = subCheck.Hashes
	p.Fingerprint = subCheck.Fingerprint
	return nil
}

func (p *PostureCheckProcess) toBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckProcess{
		OperatingSystem: p.OsType,
		Path:            p.Path,
		Hashes:          p.Hashes,
		Fingerprint:     p.Fingerprint,
	}, nil
}

func (p *PostureCheckProcess) toBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckProcess{
		OperatingSystem: p.OsType,
		Path:            p.Path,
		Hashes:          p.Hashes,
		Fingerprint:     p.Fingerprint,
	}, nil
}

func (p *PostureCheckProcess) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckProcess{
		OperatingSystem: p.OsType,
		Path:            p.Path,
		Hashes:          p.Hashes,
		Fingerprint:     p.Fingerprint,
	}, nil
}

type PostureCheckFailureValuesProcess struct {
	ActualValue   PostureResponseProcess
	ExpectedValue PostureCheckProcess
}

func (p PostureCheckFailureValuesProcess) Expected() interface{} {
	return p.ExpectedValue
}

func (p PostureCheckFailureValuesProcess) Actual() interface{} {
	return p.ActualValue
}