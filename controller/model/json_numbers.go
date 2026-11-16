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

package model

import "math"

func resolveParsedNumber(v interface{}) interface{} {
	if parsedNumber, ok := v.(ParsedNumber); ok {
		//floats don't parse as int, try int first, then float, else give up
		if intVal, err := parsedNumber.Int64(); err == nil {
			v = intVal
		} else if floatVal, err := parsedNumber.Float64(); err == nil {
			v = floatVal
			intVal := math.Trunc(floatVal)
			if intVal == floatVal {
				v = intVal
			}
		}
	}
	return v
}

func narrowJsonTypesList(l []interface{}) {
	for i, v := range l {
		v = resolveParsedNumber(v)

		switch val := v.(type) {
		case []interface{}:
			narrowJsonTypesList(val)
		case map[string]interface{}:
			NarrowJsonTypes(val)
		default:
			l[i] = v
		}
	}
}

func NarrowJsonTypes(m map[string]interface{}) {
	for k, v := range m {
		v = resolveParsedNumber(v)

		switch val := v.(type) {
		case []interface{}:
			narrowJsonTypesList(val)
		case map[string]interface{}:
			NarrowJsonTypes(val)
		default:
			m[k] = v
		}
	}
}

type ParsedNumber interface {
	String() string
	Float64() (float64, error)
	Int64() (int64, error)
}
