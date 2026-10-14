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
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
)

// PickValue prints message to stdout and returns the line read from stdin, or defaultValue if the line is empty.
// When required is set, it prompts again until the answer is non-empty. It returns an error if stdin fails or
// closes before a line is read.
func PickValue(message string, defaultValue string, required bool) (string, error) {
	return pickValue(os.Stdin, os.Stdout, message, defaultValue, required)
}

func pickValue(in io.Reader, out io.Writer, message string, defaultValue string, required bool) (string, error) {
	prompt := message + " "
	if defaultValue != "" {
		prompt = fmt.Sprintf("%s (%s) ", message, defaultValue)
	}
	for {
		_, _ = fmt.Fprint(out, prompt)
		answer, err := readLine(in)
		if err != nil {
			return "", err
		}
		if answer == "" {
			answer = defaultValue
		}
		if answer != "" || !required {
			return answer, nil
		}
		_, _ = fmt.Fprintln(out, "a value is required")
	}
}

// PickNameWithDefault prints message and a numbered list of names to stdout, then returns the entry chosen on
// stdin, by name or else by number, so a numeric name selects itself. An empty answer picks defaultValue, or the
// first entry if defaultValue is not in names. It returns "" for empty names and the only entry of a single-entry names, without prompting. It returns an
// error if stdin fails or closes before a valid choice is read.
func PickNameWithDefault(names []string, message string, defaultValue string) (string, error) {
	return pickName(os.Stdin, os.Stdout, names, message, defaultValue)
}

// PickName is PickNameWithDefault with no default, so an empty answer picks the first entry.
func PickName(names []string, message string) (string, error) {
	return PickNameWithDefault(names, message, "")
}

func pickName(in io.Reader, out io.Writer, names []string, message string, defaultValue string) (string, error) {
	if len(names) == 0 {
		return "", nil
	}
	if len(names) == 1 {
		return names[0], nil
	}

	defaultIdx := max(slices.Index(names, defaultValue), 0)

	_, _ = fmt.Fprintln(out, message)
	for i, name := range names {
		_, _ = fmt.Fprintf(out, "  %d) %s\n", i+1, name)
	}
	for {
		_, _ = fmt.Fprintf(out, "choice (%d) ", defaultIdx+1)
		answer, err := readLine(in)
		if err != nil {
			return "", err
		}
		if answer == "" {
			return names[defaultIdx], nil
		}
		if slices.Contains(names, answer) {
			return answer, nil
		}
		if idx, err := strconv.Atoi(answer); err == nil && idx >= 1 && idx <= len(names) {
			return names[idx-1], nil
		}
		_, _ = fmt.Fprintf(out, "enter a number from 1 to %d\n", len(names))
	}
}

// readLine returns the next line of in, trimmed of surrounding whitespace. It reads a byte at a time so input past
// the newline stays unread for later prompts. A final line without a newline is returned without error; an EOF
// before any input is returned as an error.
func readLine(in io.Reader) (string, error) {
	var line []byte
	b := make([]byte, 1)
	for {
		n, err := in.Read(b)
		if n > 0 {
			if b[0] == '\n' {
				break
			}
			line = append(line, b[0])
		}
		if err != nil {
			if errors.Is(err, io.EOF) && len(line) > 0 {
				break
			}
			return "", fmt.Errorf("reading answer: %w", err)
		}
	}
	return strings.TrimSpace(string(line)), nil
}
