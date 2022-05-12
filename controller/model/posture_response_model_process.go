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
	"strings"
	"time"
)

type PostureResponseProcess struct {
	*PostureResponse
	Path               string
	IsRunning          bool
	BinaryHash         string
	SignerFingerprints []string
}

func (pr *PostureResponseProcess) Apply(postureData *PostureData) {
	found := false

	for i, fingerprint := range pr.SignerFingerprints {
		pr.SignerFingerprints[i] = CleanHexString(fingerprint)
	}

	pr.BinaryHash = CleanHexString(pr.BinaryHash)

	for i, process := range postureData.Processes {
		if process.PostureCheckId == pr.PostureCheckId {
			postureData.Processes[i] = pr
			postureData.Processes[i].LastUpdatedAt = time.Now()
			found = true
			break
		}
	}

	if !found {
		pr.LastUpdatedAt = time.Now().UTC()
		postureData.Processes = append(postureData.Processes, pr)
	}

	if pr.Path != "" {
		postureData.ProcessPathMap[pr.Path] = pr
	}
}

func (pr *PostureResponseProcess) VerifyMultiCriteria(process *ProcessMulti) bool {
	if !pr.IsRunning {
		return false
	}

	if pr.TimedOut {
		return false
	}

	foundValidHash := false

	if len(process.Hashes) == 0 {
		foundValidHash = true //no hash to check for
	} else {
		for _, validHash := range process.Hashes {
			if strings.ToLower(validHash) == strings.ToLower(pr.BinaryHash) {
				foundValidHash = true
				break
			}
		}
	}

	if !foundValidHash {
		return false
	}

	foundValidSigner := false

	if len(process.SignerFingerprints) == 0 {
		foundValidSigner = true //no signerByIssuer to check for
	} else {
		for _, validSigner := range process.SignerFingerprints {
			validSigner := strings.ToLower(validSigner)
			for _, signer := range pr.SignerFingerprints {
				if strings.ToLower(signer) == validSigner {
					foundValidSigner = true
					break
				}
			}

			if foundValidSigner {
				break
			}
		}
	}

	if !foundValidSigner {
		return false
	}

	return true
}
