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

package util

import (
	"fmt"
	"github.com/openziti/ziti/ziti/internal/log"
	"sort"
	"strings"

	"gopkg.in/AlecAivazis/survey.v1"
)

func PickValue(message string, defaultValue string, required bool) (string, error) {
	answer := ""
	prompt := &survey.Input{
		Message: message,
		Default: defaultValue,
	}
	validator := survey.Required
	if !required {
		validator = nil
	}
	err := survey.AskOne(prompt, &answer, validator)
	if err != nil {
		return "", err
	}
	return answer, nil
}

func PickPassword(message string) (string, error) {
	answer := ""
	prompt := &survey.Password{
		Message: message,
	}
	validator := survey.Required
	err := survey.AskOne(prompt, &answer, validator)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(answer), nil
}

func PickNameWithDefault(names []string, message string, defaultValue string) (string, error) {
	name := ""
	if len(names) == 0 {
		return "", nil
	} else if len(names) == 1 {
		name = names[0]
	} else {
		prompt := &survey.Select{
			Message: message,
			Options: names,
			Default: defaultValue,
		}
		err := survey.AskOne(prompt, &name, nil)
		if err != nil {
			return "", err
		}
	}
	return name, nil
}

func PickRequiredNameWithDefault(names []string, message string, defaultValue string) (string, error) {
	name := ""
	if len(names) == 0 {
		return "", nil
	} else if len(names) == 1 {
		name = names[0]
	} else {
		prompt := &survey.Select{
			Message: message,
			Options: names,
			Default: defaultValue,
		}
		err := survey.AskOne(prompt, &name, survey.Required)
		if err != nil {
			return "", err
		}
	}
	return name, nil
}

func PickName(names []string, message string) (string, error) {
	return PickNameWithDefault(names, message, "")
}

func PickNames(names []string, message string) ([]string, error) {
	picked := []string{}
	if len(names) == 0 {
		return picked, nil
	} else if len(names) == 1 {
		return names, nil
	} else {
		prompt := &survey.MultiSelect{
			Message: message,
			Options: names,
		}
		err := survey.AskOne(prompt, &picked, nil)
		if err != nil {
			return picked, err
		}
	}
	return picked, nil
}

// SelectNamesWithFilter selects from a list of names with a given filter. Optionally selecting them all
func SelectNamesWithFilter(names []string, message string, selectAll bool, filter string) ([]string, error) {
	filtered := []string{}
	for _, name := range names {
		if filter == "" || strings.Contains(name, filter) {
			filtered = append(filtered, name)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("No names match filter: %s", filter)
	}
	return SelectNames(filtered, message, selectAll)
}

// SelectNames select which names from the list should be chosen
func SelectNames(names []string, message string, selectAll bool) ([]string, error) {
	answer := []string{}
	if len(names) == 0 {
		return answer, fmt.Errorf("No names to choose from!")
	}
	sort.Strings(names)

	prompt := &survey.MultiSelect{
		Message: message,
		Options: names,
	}
	if selectAll {
		prompt.Default = names
	}
	err := survey.AskOne(prompt, &answer, nil)
	return answer, err
}

// Confirm prompts the user to confirm something
func Confirm(message string, defaultValue bool, help string) bool {
	answer := defaultValue
	prompt := &survey.Confirm{
		Message: message,
		Default: defaultValue,
		Help:    help,
	}
	_ = survey.AskOne(prompt, &answer, nil)
	log.Blank()
	return answer
}
