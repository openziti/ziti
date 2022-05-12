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
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestPostureCheckModelProcess_Evaluate(t *testing.T) {

	t.Run("returns true for valid id, running, hash, and fingerprint", func(t *testing.T) {
		processCheck, postureData := newMatchingProcessCheckAndData()

		result := processCheck.Evaluate("", postureData)

		req := require.New(t)
		req.True(result)
	})

	t.Run("returns false if not running", func(t *testing.T) {
		processCheck, postureData := newMatchingProcessCheckAndData()
		postureData.Processes[0].IsRunning = false

		result := processCheck.Evaluate("", postureData)
		req := require.New(t)

		req.False(result)
	})

	t.Run("returns true for valid id, running, hash, and fingerprint with mismatched hash case", func(t *testing.T) {
		processCheck, postureData := newMatchingProcessCheckAndData()
		postureData.Processes[0].BinaryHash = strings.ToUpper(postureData.Processes[0].BinaryHash)

		result := processCheck.Evaluate("", postureData)
		req := require.New(t)

		req.True(result)
	})

	t.Run("returns true for valid id, running, hash, and fingerprint with mismatched signer case", func(t *testing.T) {
		processCheck, postureData := newMatchingProcessCheckAndData()
		postureData.Processes[0].SignerFingerprints[0] = strings.ToUpper(postureData.Processes[0].SignerFingerprints[0])

		result := processCheck.Evaluate("", postureData)
		req := require.New(t)

		req.True(result)
	})

	t.Run("returns true for valid id and running but check has null hashes and no signer", func(t *testing.T) {
		processCheck, postureData := newMatchingProcessCheckAndData()
		processCheck.Hashes = nil
		processCheck.Fingerprint = ""

		result := processCheck.Evaluate("", postureData)

		req := require.New(t)
		req.True(result)
	})

	t.Run("returns true for valid id and running but check has empty hashes and no signer", func(t *testing.T) {
		processCheck, postureData := newMatchingProcessCheckAndData()
		processCheck.Hashes = []string{}
		processCheck.Fingerprint = ""

		result := processCheck.Evaluate("", postureData)

		req := require.New(t)
		req.True(result)
	})

	t.Run("returns true for valid id, running, signer but check has no hash", func(t *testing.T) {
		processCheck, postureData := newMatchingProcessCheckAndData()
		processCheck.Hashes = nil

		result := processCheck.Evaluate("", postureData)

		req := require.New(t)
		req.True(result)
	})

	t.Run("returns true for valid id, running, hashes but check has no signer", func(t *testing.T) {
		processCheck, postureData := newMatchingProcessCheckAndData()
		processCheck.Fingerprint = ""

		result := processCheck.Evaluate("", postureData)

		req := require.New(t)
		req.True(result)
	})

	t.Run("returns false if ids do not match", func(t *testing.T) {
		processCheck, postureData := newMatchingProcessCheckAndData()
		processCheck.PostureCheckId = "does not match"

		result := processCheck.Evaluate("", postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns false if signerByIssuer do not match", func(t *testing.T) {
		processCheck, postureData := newMatchingProcessCheckAndData()
		postureData.Processes[0].SignerFingerprints = []string{"does not match"}

		result := processCheck.Evaluate("", postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns false if hashes do not match", func(t *testing.T) {
		processCheck, postureData := newMatchingProcessCheckAndData()
		postureData.Processes[0].BinaryHash = "does not match"

		result := processCheck.Evaluate("", postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns false not running, invalid hash, invalid signer", func(t *testing.T) {
		processCheck, postureData := newMatchingProcessCheckAndData()
		postureData.Processes[0].IsRunning = false
		postureData.Processes[0].BinaryHash = "does not match"
		postureData.Processes[0].SignerFingerprints = []string{"does not match"}

		result := processCheck.Evaluate("", postureData)

		req := require.New(t)
		req.False(result)
	})
}

// Returns a process check and posture data that will pass with matching id, hash, signer, and running state. Can
// be altered to test various pass/fail states
func newMatchingProcessCheckAndData() (*PostureCheckProcess, *PostureData) {
	postureCheckId := "30qhj45"
	binaryHash := "b4f3228217a2bae3f21f6b6df3750d0723a5c3973db9aad360a8f25bc31e3676d38180cf0abc89d7fca7a26e1919a1e52739ed3116011acc7e96630313da56b8"
	signerFingerprint := "950248b9e8b0dd41938018a871a13dd92bed4614"

	postureResponse := &PostureResponse{
		PostureCheckId: postureCheckId,
		TypeId:         PostureCheckTypeProcess,
		TimedOut:       false,
		LastUpdatedAt:  time.Now(),
	}

	postureResponseProcess := &PostureResponseProcess{
		IsRunning:          true,
		BinaryHash:         binaryHash,
		SignerFingerprints: []string{signerFingerprint},
	}

	postureResponse.SubType = postureResponseProcess
	postureResponseProcess.PostureResponse = postureResponse

	validPostureData := &PostureData{
		Processes: []*PostureResponseProcess{
			postureResponseProcess,
		},
	}

	processCheck := &PostureCheckProcess{
		PostureCheckId: postureCheckId,
		OsType:         "Windows",
		Path:           `C:\some\path\some.exe`,
		Hashes: []string{
			"something that will never match 1",
			binaryHash,
			"something that will never match 2",
		},
		Fingerprint: signerFingerprint,
	}

	return processCheck, validPostureData
}
