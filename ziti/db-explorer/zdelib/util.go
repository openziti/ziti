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

package zdelib

import (
	"errors"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
	"os"
	"time"
)

// TypeToString will convert boltz types to string values. If an unknown type is encountered, "unknown" is returned.
func TypeToString(fieldType boltz.FieldType) string {
	switch fieldType {
	case boltz.TypeString:
		return "string"
	case boltz.TypeInt32:
		return "int32"
	case boltz.TypeTime:
		return "date.Time"
	case boltz.TypeBool:
		return "bool"
	case boltz.TypeFloat64:
		return "float64"
	case boltz.TypeInt64:
		return "int65"
	case boltz.TypeNil:
		return "nil"
	}

	return "unknown"
}

// Open attempts to open the provided path as a bbolt.DB. Returns a bbolt.Db or an error
func Open(path string) (*bbolt.DB, error) {
	fileInfo, err := os.Stat(path)

	if err != nil {
		return nil, err
	}

	if fileInfo.IsDir() {
		return nil, errors.New("path must be to a file")
	}

	return bbolt.Open(path, 0666, &bbolt.Options{
		Timeout:  10 * time.Second,
		ReadOnly: true,
	})
}
