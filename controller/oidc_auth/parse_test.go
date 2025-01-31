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

package oidc_auth

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_Map(t *testing.T) {

	const (
		id            = "123"
		envArch       = "x64"
		envOs         = "linux"
		envOsRelease  = "11"
		envOsVersion  = "1.1.1"
		sdkAppId      = "myAppid"
		sdkAppVersion = "5.1.1"
		sdkBranch     = "branch34"
		sdkRevision   = "rev1"
		sdkType       = "fakeType"
		sdkVersion    = "6.4.3"
		username      = "admin"
		password      = "fake_admin_password"
		configType1   = "one"
		configType2   = "two"
	)

	t.Run("totp id/code payload", func(t *testing.T) {
		dstTotp := &Totp{}

		srcMap := map[string][]string{
			"id":   {"123"},
			"code": {"456"},
		}
		err := MapToStruct(srcMap, dstTotp)

		req := require.New(t)
		req.NoError(err)
		req.Equal(dstTotp.Code, "456")
		req.Equal(dstTotp.AuthRequestId, "123")
	})

	t.Run("updb creds with env/sdk info payload", func(t *testing.T) {
		srcMap := map[string][]string{
			"id":            {id},
			"envArch":       {envArch},
			"envOs":         {envOs},
			"envOsRelease":  {envOsRelease},
			"envOsVersion":  {envOsVersion},
			"sdkAppId":      {sdkAppId},
			"sdkAppVersion": {sdkAppVersion},
			"sdkBranch":     {sdkBranch},
			"sdkRevision":   {sdkRevision},
			"sdkType":       {sdkType},
			"sdkVersion":    {sdkVersion},
			"username":      {username},
			"password":      {password},
			"configTypes":   {"one", "two"},
		}

		dst := &updbCreds{}

		err := MapToStruct(srcMap, dst)

		req := require.New(t)
		req.NoError(err)
		req.Equal(dst.AuthRequestId, id)
		req.NotNil(dst.EnvInfo)
		req.Equal(dst.EnvInfo.Arch, envArch)
		req.Equal(dst.EnvInfo.OsVersion, envOsVersion)
		req.Equal(dst.EnvInfo.OsRelease, envOsRelease)
		req.Equal(dst.EnvInfo.Os, envOs)
		req.NotNil(dst.SdkInfo)
		req.Equal(dst.SdkInfo.AppID, sdkAppId)
		req.Equal(dst.SdkInfo.AppVersion, sdkAppVersion)
		req.Equal(dst.SdkInfo.Type, sdkType)
		req.Equal(dst.SdkInfo.Revision, sdkRevision)
		req.Equal(dst.SdkInfo.Branch, sdkBranch)
		req.Equal(string(dst.Username), username)
		req.Equal(string(dst.Password), password)
		req.NotNil(dst.ConfigTypes)
		req.Equal(dst.ConfigTypes[0], configType1)
		req.Equal(dst.ConfigTypes[1], configType2)
	})

	// test - operator
	// test nil translator
	// test bad field names in translator
}
