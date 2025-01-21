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

package config

import (
	"github.com/michaelquigley/pfxlog"
	"os"
	"sync"
)

var ensureTmpDirEnvOnce sync.Once

func EnsureTempDirEnv() {
	defaultTemp := os.TempDir()

	err := os.Setenv("TMP", defaultTemp)

	if err != nil {
		pfxlog.Logger().WithError(err).Warn("Could not set TMP environment variable")
	}

	err = os.Setenv("TEMP", defaultTemp)

	if err != nil {
		pfxlog.Logger().WithError(err).Warn("Could not set TEMP environment variable")
	}

	err = os.Setenv("TMPDIR", defaultTemp)

	if err != nil {
		pfxlog.Logger().WithError(err).Warn("Could not set TMPDIR environment variable")
	}
}

func InjectEnv(config map[interface{}]interface{}) {
	ensureTmpDirEnvOnce.Do(EnsureTempDirEnv)
	
	for key, v := range config {
		if str, ok := v.(string); ok {
			config[key] = os.ExpandEnv(str)
		} else if m, ok := v.(map[interface{}]interface{}); ok {
			InjectEnv(m)
		} else if s, ok := v.([]interface{}); ok {
			InjectEnvSlice(s)
		}
	}
}

func InjectEnvSlice(slice []interface{}) {
	for idx, v := range slice {
		if str, ok := v.(string); ok {
			slice[idx] = os.ExpandEnv(str)
		} else if m, ok := v.(map[interface{}]interface{}); ok {
			InjectEnv(m)
		} else if s, ok := v.([]interface{}); ok {
			InjectEnvSlice(s)
		}
	}
}
