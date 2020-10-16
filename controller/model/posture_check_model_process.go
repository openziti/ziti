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

type PostureCheckProcess struct {
	PostureCheckId  string
	OperatingSystem string
	Path            string
	Hashes          []string
	Fingerprint     string
}

func (p *PostureCheckProcess) Evaluate(pd *PostureData) bool {
	for _, process := range pd.Processes {
		if process.PostureCheckId == p.PostureCheckId {
			if process.TimedOut {
				return false
			}

			if p.Fingerprint != "" {
				if p.Fingerprint != process.SignerFingerprint {
					return false
				}
			}

			if len(p.Hashes) > 0 {
				for _, hash := range p.Hashes {
					if hash == process.BinaryHash {
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
	p.OperatingSystem = subCheck.OperatingSystem
	p.Path = subCheck.Path
	p.Hashes = subCheck.Hashes
	p.Fingerprint = subCheck.Fingerprint
	return nil
}

func (p *PostureCheckProcess) toBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckProcess{
		OperatingSystem: p.OperatingSystem,
		Path:            p.Path,
		Hashes:          p.Hashes,
		Fingerprint:     p.Fingerprint,
	}, nil
}

func (p *PostureCheckProcess) toBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckProcess{
		OperatingSystem: p.OperatingSystem,
		Path:            p.Path,
		Hashes:          p.Hashes,
		Fingerprint:     p.Fingerprint,
	}, nil
}

func (p *PostureCheckProcess) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckProcess{
		OperatingSystem: p.OperatingSystem,
		Path:            p.Path,
		Hashes:          p.Hashes,
		Fingerprint:     p.Fingerprint,
	}, nil
}
