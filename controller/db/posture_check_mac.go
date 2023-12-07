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

package db

import (
	"github.com/openziti/storage/boltz"
	"regexp"
	"strings"
)

const (
	FieldPostureCheckMacAddresses = "macAddresses"
)

func newPostureCheckMacAddresses() PostureCheckSubType {
	return &PostureCheckMacAddresses{
		MacAddresses: []string{},
	}
}

type PostureCheckMacAddresses struct {
	MacAddresses []string `json:"macAddresses"`
}

func (entity *PostureCheckMacAddresses) LoadValues(bucket *boltz.TypedBucket) {
	for _, macAddress := range bucket.GetStringList(FieldPostureCheckMacAddresses) {
		macAddress = cleanMacAddress(macAddress)
		entity.MacAddresses = append(entity.MacAddresses, macAddress)
	}
}

func (entity *PostureCheckMacAddresses) SetValues(ctx *boltz.PersistContext, bucket *boltz.TypedBucket) {
	var macAddresses []string
	for _, macAddress := range entity.MacAddresses {
		macAddress = cleanMacAddress(macAddress)
		macAddresses = append(macAddresses, macAddress)
	}
	entity.MacAddresses = macAddresses
	bucket.SetStringList(FieldPostureCheckMacAddresses, macAddresses, ctx.FieldChecker)
}

func cleanMacAddress(macAddress string) string {
	macAddress = strings.ToLower(macAddress)
	nonHex := regexp.MustCompile("[^a-f0-9]")
	return nonHex.ReplaceAllString(macAddress, "")
}
