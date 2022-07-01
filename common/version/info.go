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

package version

import (
	"github.com/openziti/foundation/v2/versions"
	"runtime"
)

type cmdBuildInfo struct{}

func (c cmdBuildInfo) EncoderDecoder() versions.VersionEncDec {
	return &versions.StdVersionEncDec
}

func (c cmdBuildInfo) Version() string {
	return Version
}

func (c cmdBuildInfo) Revision() string {
	return Revision
}

func (c cmdBuildInfo) BuildDate() string {
	return BuildDate
}

func (c cmdBuildInfo) Branch() string {
	return Branch
}

func (c cmdBuildInfo) AsVersionInfo() *versions.VersionInfo {
	return &versions.VersionInfo{
		Version:   c.Version(),
		Revision:  c.Revision(),
		BuildDate: c.BuildDate(),
		OS:        GetOS(),
		Arch:      GetArchitecture(),
	}
}

func GetCmdBuildInfo() versions.VersionProvider {
	return cmdBuildInfo{}
}

func GetBuildMetadata(verbose bool) string {
	if !verbose {
		return GetVersion()
	}
	str :=
		"\n\t" + "Version:    " + GetVersion() +
			"\n\t" + "Build Date: " + GetBuildDate() +
			"\n\t" + "Git Branch: " + GetBranch() +
			"\n\t" + "Git SHA:    " + GetRevision() +
			"\n\t" + "Go Version: " + GetGoVersion() +
			"\n\t" + "OS:         " + GetOS() +
			"\n\t" + "Arch:       " + GetArchitecture() +
			"\n"

	return str
}

func GetVersion() string {
	return Version
}

func GetRevision() string {
	return Revision
}

func GetBranch() string {
	return Branch
}

func GetBuildDate() string {
	return BuildDate
}

func GetGoVersion() string {
	return runtime.Version()
}

func GetOS() string {
	return runtime.GOOS
}

func GetArchitecture() string {
	return runtime.GOARCH
}
