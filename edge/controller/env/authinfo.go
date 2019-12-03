/*
	Copyright 2019 Netfoundry, Inc.

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

package env

import (
	"github.com/Jeffail/gabs"
)

type EnvInfo struct {
	Arch      string `json:"arch"`
	Os        string `json:"os"`
	OsRelease string `json:"osRelease"`
	OsVersion string `json:"osVersion"`
}

type SdkInfo struct {
	Branch   string `json:"branch"`
	Revision string `json:"revision"`
	Type     string `json:"type"`
	Version  string `json:"version"`
}

type Info struct {
	EnvInfo *EnvInfo `json:"envInfo"`
	SdkInfo *SdkInfo `json:"sdkInfo"`
}

func ParseInfoFromBody(body []byte) (map[interface{}]interface{}, error) {
	container, err := gabs.ParseJSON(body)
	if err != nil {
		return nil, err
	}
	return ParseInfoFromContainer(container)
}

func ParseInfoFromContainer(container *gabs.Container) (map[interface{}]interface{}, error) {
	info := map[interface{}]interface{}{}

	envInfo := container.Search("envInfo")
	sdkInfo := container.Search("sdkInfo")

	info["envInfo"] = envInfo.Data()
	info["sdkInfo"] = sdkInfo.Data()

	return info, nil
}
