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

package enrollment

import (
	"errors"
	"fmt"
	"strings"
)

type keyTypeVar string

func (f *keyTypeVar) String() string {
	return fmt.Sprint(string(*f))
}

func (f *keyTypeVar) Set(value string) error {
	value = strings.ToUpper(value)
	if value != "EC" && value != "RSA" {
		return errors.New("Invalid option -- must specify either 'EC' or 'RSA'")
	}
	*f = keyTypeVar(value)
	return nil
}

func (f *keyTypeVar) EC() bool {
	if string(*f) == "EC" {
		return true
	}
	return false
}

func (f *keyTypeVar) RSA() bool {
	if string(*f) == "RSA" {
		return true
	}
	return false
}

func (f *keyTypeVar) Get() string {
	return string(*f)
}

func (f *keyTypeVar) Type() string {
	return "EC|RSA"
}
