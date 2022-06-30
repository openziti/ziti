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

package edge

import (
	"errors"
	"fmt"
	"strings"
)

type encryptionVar string

func (f *encryptionVar) String() string {
	return fmt.Sprint(string(*f))
}

func (f *encryptionVar) Set(value string) error {
	value = strings.ToUpper(value)
	if value != "ON" && value != "OFF" && value != "TRUE" && value != "FALSE" {
		return errors.New("Invalid option -- must specify either 'ON' or 'OFF'")
	}
	*f = encryptionVar(value)
	return nil
}

func (f *encryptionVar) Get() bool {
	if string(*f) == "ON" || string(*f) == "TRUE" {
		return true
	}
	return false
}

func (f *encryptionVar) Type() string {
	return "ON|OFF"
}
