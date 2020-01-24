/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/blang/semver"
	"log"
	"strings"
)

// Build information. Populated at build-time.
var (
	Verbose   bool
	Version   string
	Revision  string
	Branch    string
	BuildUser string
	BuildDate string
	GoVersion string
	OS        string
	Arch      string
)

// Map provides the iterable version information.
var Map = map[string]string{
	"version":   Version,
	"revision":  Revision,
	"branch":    Branch,
	"buildUser": BuildUser,
	"buildDate": BuildDate,
	"goVersion": GoVersion,
	"os":        OS,
	"arch":      Arch,
}

const (
	VersionPrefix = ""

	// TestVersion used in test cases for the current version if no
	// version can be found - such as if the version property is not properly
	// included in the go test flags
	TestVersion = "0.0.0"

	TestRevision  = "unknown"
	TestBranch    = "unknown"
	TestBuildDate = "unknown"
	TestGoVersion = "unknown"
	TestOs        = "unknown"
	TestArch      = "unknown"
)

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
	v := Map["version"]
	if v == "" {
		v = TestVersion
	}
	return v
}

func GetRevision() string {
	r := Map["revision"]
	if r == "" {
		r = TestRevision
	}
	return r
}

func GetBranch() string {
	b := Map["branch"]
	if b == "" {
		b = TestBranch
	}
	return b
}

func GetBuildDate() string {
	d := Map["buildDate"]
	if d == "" {
		d = TestBuildDate
	}
	return d
}

func GetGoVersion() string {
	g := Map["goVersion"]
	if g == "" {
		g = TestRevision
	}
	return g
}

func GetOS() string {
	o := Map["os"]
	if o == "" {
		o = TestRevision
	}
	return o
}

func GetArchitecture() string {
	a := Map["arch"]
	if a == "" {
		a = TestRevision
	}
	return a
}

func GetSemverVersion() (semver.Version, error) {
	return semver.Make(strings.TrimPrefix(GetVersion(), VersionPrefix))
}

// VersionStringDefault returns the current version string or returns a dummy
// default value if there is an error
func VersionStringDefault(defaultValue string) string {
	v, err := GetSemverVersion()
	if err == nil {
		return v.String()
	}
	log.Printf("Warning failed to load version: %s\n", err)
	return defaultValue
}
