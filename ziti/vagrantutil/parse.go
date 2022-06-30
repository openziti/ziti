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

package vagrantutil

import (
	"bufio"
	"fmt"
	"strings"
)

// parseData parses the given vagrant type field from the machine readable
// output (records).
func parseData(records [][]string, typeName string) (string, error) {
	data := ""
	for _, record := range records {
		// first three are defined, after that data is variadic, it contains
		// zero or more information. We should have a data, otherwise it's
		// useless.
		if len(record) < 4 {
			continue
		}

		if typeName == record[2] && record[3] != "" {
			data = record[3]
			break
		}
	}

	if data == "" {
		return "", fmt.Errorf("couldn't parse data for vagrant type: %q", typeName)
	}

	return data, nil
}

func parseRecords(out string) (recs [][]string, err error) {
	scanner := bufio.NewScanner(strings.NewReader(out))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		row := strings.Split(line, ",")
		recs = append(recs, row)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return recs, nil
}
