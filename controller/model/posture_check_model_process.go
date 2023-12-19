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

var _ PostureCheckSubType = &PostureCheckProcess{}

type PostureCheckProcess struct {
	PostureCheckId string
	OsType         string
	Path           string
	Hashes         []string
	Fingerprint    string
}

func (p *PostureCheckProcess) TypeId() string {
	return db.PostureCheckTypeProcess
}

func (p *PostureCheckProcess) fillProtobuf(msg *edge_cmd_pb.PostureCheck) {
	msg.Subtype = &edge_cmd_pb.PostureCheck_Process_{
		Process: &edge_cmd_pb.PostureCheck_Process{
			OsType:       p.OsType,
			Path:         p.Path,
			Hashes:       p.Hashes,
			Fingerprints: []string{p.Fingerprint},
		},
	}
}

func (p *PostureCheckProcess) fillFromProtobuf(msg *edge_cmd_pb.PostureCheck) error {
	if process_, ok := msg.Subtype.(*edge_cmd_pb.PostureCheck_Process_); ok {
		if process := process_.Process; process != nil {
			p.PostureCheckId = msg.Id
			p.OsType = process.OsType
			p.Path = process.Path
			p.Hashes = process.Hashes

			var fingerprint string
			if len(process.Fingerprints) > 0 {
				fingerprint = process.Fingerprints[0]
			}
			p.Fingerprint = fingerprint
		}
	} else {
		return errors.Errorf("expected posture check sub type data of process, but got %T", msg.Subtype)
	}
	return nil
}

func (p *PostureCheckProcess) LastUpdatedAt(id string, pd *PostureData) *time.Time {
	return nil
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
			break
		}
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

func (p *PostureCheckProcess) fillFrom(_ Env, tx *bbolt.Tx, check *db.PostureCheck, subType db.PostureCheckSubType) error {
	subCheck := subType.(*db.PostureCheckProcess)

	if subCheck == nil {
		return fmt.Errorf("could not convert process check to bolt type")
	}

	p.PostureCheckId = check.Id
	p.OsType = subCheck.OperatingSystem
	p.Path = subCheck.Path
	p.Hashes = subCheck.Hashes
	p.Fingerprint = subCheck.Fingerprint
	return nil
}

func (p *PostureCheckProcess) toBoltEntityForCreate(*bbolt.Tx, Env) (db.PostureCheckSubType, error) {
	return &db.PostureCheckProcess{
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
