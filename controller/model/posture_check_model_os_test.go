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
	"testing"
	"time"
)

func TestPostureCheckModelOs_Evaluate(t *testing.T) {

	t.Run("returns true for exactly matching valid os type and version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.True(result)
	})

	t.Run("returns true for valid os type and higher than required major version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		osCheck.OperatingSystems[0].OsVersions[0] = ">=1.0.0"

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.True(result)
	})

	t.Run("returns true for valid os type and higher than required minor version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		osCheck.OperatingSystems[0].OsVersions[0] = ">=10.4.0"

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.True(result)
	})

	t.Run("returns true for valid os type and higher than required patch version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		osCheck.OperatingSystems[0].OsVersions[0] = ">=10.5.0"

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.True(result)
	})

	t.Run("returns false for valid os type and lower than required major version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		osCheck.OperatingSystems[0].OsVersions[0] = ">=11.0.0" //default data has 10.5.19041

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns false for valid os type and lower than required minor version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		osCheck.OperatingSystems[0].OsVersions[0] = ">10.6.0" //default data has 10.5.19041

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns false for valid os type and lower than required patch version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		osCheck.OperatingSystems[0].OsVersions[0] = ">=10.5.19042" //default data has 10.5.19041

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns true for valid os type exactly matching version or higher", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		osCheck.OperatingSystems[0].OsVersions[0] = ">=10.5.19041" //default data has 10.5.19041

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.True(result)
	})

	t.Run("returns false for invalid os but exactly matching version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		osCheck.OperatingSystems[0].OsType = os2Type

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns false for valid os and no matching version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		postureData.Os.Type = os2Type
		postureData.Os.Version = ""

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns true for valid os and no required version and no submitted version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		postureData.Os.Type = os3Type
		postureData.Os.Version = ""

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.True(result)
	})

	t.Run("returns true for valid os and no required version and submitted version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		postureData.Os.Type = os3Type
		postureData.Os.Version = "1.2.3"

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.True(result)
	})

	t.Run("returns true for lower exact version match and later and higher match", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		postureData.Os.Type = os2Type
		postureData.Os.Version = os2Version1

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.True(result)
	})

	t.Run("returns true for higher exact version match and an earlier match", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		postureData.Os.Type = os2Type
		postureData.Os.Version = "7.8.9"

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.True(result)
	})

	//poorly formatted posture data
	t.Run("returns false for posture data with valid os and partial os major version match", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		postureData.Os.Version = "10"

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns false for posture data with valid os and partial os major and minor version match", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		postureData.Os.Version = "10.5"

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns false for posture data with valid os and partial os major version match with dangling period", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		postureData.Os.Version = "10."

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns false for posture data with valid os and partial os major and minor version match with dangling period", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		postureData.Os.Version = "10.5."

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.False(result)
	})


	t.Run("returns false for posture data with valid os and partial os major and dangling periods", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		postureData.Os.Version = "10.5."

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns false for posture data with valid os and empty version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		postureData.Os.Version = ""

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns false for posture data with valid os and partially valid and mangled version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		postureData.Os.Version = "10.this is not a real version"

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.False(result)
	})

	t.Run("returns false for posture data with valid os and fully mangled version", func(t *testing.T) {
		osCheck, postureData := newMatchingOsCheckAndData()
		postureData.Os.Version = "this is not a real version"

		result := osCheck.Evaluate(postureData)

		req := require.New(t)
		req.False(result)
	})
}


const (
	os1Type    = "Windows"
	os1Version = "10.5.19041"

	os2Type     = "Linux"
	os2Version1 = "3.4.5"
	os2Version2 = ">=7.8.9"

	os3Type = "Android"
)

// Returns a process check and posture data that will pass with matching os type, version, and build. Can
// be altered to test various pass/fail states
func newMatchingOsCheckAndData() (*PostureCheckOperatingSystem, *PostureData) {
	postureCheckId := "30qhj45"

	postureResponse := &PostureResponse{
		PostureCheckId: postureCheckId,
		TypeId:         PostureCheckTypeOs,
		TimedOut:       false,
		LastUpdatedAt:  time.Now(),
	}

	postureResponseOs := &PostureResponseOs{
		PostureResponse: nil,
		Type:            os1Type,
		Version:         os1Version,
	}

	postureResponse.SubType = postureResponseOs
	postureResponseOs.PostureResponse = postureResponse

	validPostureData := &PostureData{
		Mac:       nil,
		Domain:    nil,
		Os:        postureResponseOs,
		Processes: nil,
	}

	processCheck := &PostureCheckOperatingSystem{
		OperatingSystems: []OperatingSystem{
			{
				OsType:     os1Type,
				OsVersions: []string{os1Version},
			},
			{
				OsType: os2Type,
				OsVersions: []string{
					os2Version1,
					os2Version2,
				},
			},
			{
				OsType:     os3Type,
				OsVersions: nil,
			},
		},
	}

	return processCheck, validPostureData
}
