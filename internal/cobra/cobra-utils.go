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

package cobra

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func AddFlagAnnotation(cmd *cobra.Command, flagName string, key string, value string) error {
	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		return fmt.Errorf("flag %q not found", flagName)
	}
	if flag.Annotations == nil {
		flag.Annotations = make(map[string][]string)
	}
	flag.Annotations[key] = append(flag.Annotations[key], value)
	return nil
}

func GetFlagsForAnnotation(cmd *cobra.Command, annotation string) string {
	var result string
	maxLength := MaxFlagNameLength(cmd)
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Annotations[annotation] != nil {
			if flag.Shorthand != "" {
				result += fmt.Sprintf("  -%s, --%-*s %s\n", flag.Shorthand, maxLength, flag.Name, flag.Usage)
			} else {
				result += fmt.Sprintf("      --%-*s %s\n", maxLength, flag.Name, flag.Usage)
			}
		}
	})

	return result
}

func GetFlagsWithoutAnnotations(cmd *cobra.Command, annotations ...string) string {
	var result string
	maxLength := MaxFlagNameLength(cmd)

	// Create a map of the provided annotations for quick lookup
	annotationMap := make(map[string]bool)
	for _, annotation := range annotations {
		annotationMap[annotation] = true
	}

	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		hasAnnotation := false
		for ann := range flag.Annotations {
			if annotationMap[ann] {
				hasAnnotation = true
				break
			}
		}
		if !hasAnnotation {
			if flag.Shorthand != "" {
				result += fmt.Sprintf("  -%s, --%-*s %s\n", flag.Shorthand, maxLength, flag.Name, flag.Usage)
			} else {
				result += fmt.Sprintf("      --%-*s %s\n", maxLength, flag.Name, flag.Usage)
			}
		}
	})

	return result
}

func MaxFlagNameLength(cmd *cobra.Command) int {
	// Calculate the maximum flag length across ALL flags
	maxLength := 0
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		length := len(flag.Name)
		if flag.Shorthand != "" {
			length += 4 // Account for "-x, "
		}
		if length > maxLength {
			maxLength = length
		}
	})
	return maxLength
}
