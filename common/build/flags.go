package build

import (
	"regexp"
	"strings"
)

const (
	// maxBuildFlags is the greatest number of flag names accepted from buildFlags.
	maxBuildFlags = 32

	// maxBuildFlagNameLength is the greatest length accepted for a single flag name.
	maxBuildFlagNameLength = 64
)

// buildFlagNamePattern matches a well formed build flag name.
var buildFlagNamePattern = regexp.MustCompile(`^[A-Z0-9_]+$`)

// buildFlags is a comma separated list of flag names supplied at build time by the linker, empty
// in a stock build:
//
//	-X github.com/openziti/ziti/v2/common/build.buildFlags=ALPHA,BRAVO
//
// The symbol path is a contract with downstream builds. The linker silently ignores -X against a
// symbol it cannot resolve, so renaming this variable or moving it to another package produces a
// binary with no build flags rather than a build error. A second -X against this symbol replaces
// the first rather than adding to it: one build owner composes the whole list.
var buildFlags string

// GetBuildFlags returns the well formed flag names supplied at build time, in the order they were
// given. It returns an empty slice for a stock build.
func GetBuildFlags() []string {
	return parseBuildFlags(buildFlags)
}

// parseBuildFlags splits raw on commas and returns the well formed names in first seen order,
// dropping blanks, duplicates, and anything outside [A-Z0-9_]. The result is never nil, so it
// serializes as an empty JSON array rather than null. The count of names and the length of each
// are capped, because the result is served over an unauthenticated API.
func parseBuildFlags(raw string) []string {
	result := []string{}
	seen := map[string]struct{}{}

	for _, token := range strings.Split(raw, ",") {
		if len(result) == maxBuildFlags {
			break
		}

		name := strings.TrimSpace(token)
		if len(name) > maxBuildFlagNameLength || !buildFlagNamePattern.MatchString(name) {
			continue
		}

		if _, ok := seen[name]; ok {
			continue
		}

		seen[name] = struct{}{}
		result = append(result, name)
	}

	return result
}
