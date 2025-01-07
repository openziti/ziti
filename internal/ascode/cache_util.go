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

package ascode

import (
	"errors"
	log "github.com/sirupsen/logrus"
	"reflect"
)

type CacheGetter func(id string) (interface{}, error)

func GetItemFromCache(c map[string]interface{}, key string, fn CacheGetter) (interface{}, error) {
	if key == "" {
		return nil, errors.New("key is null, can't resolve from cache or get it from source")
	}
	detail, found := c[key]
	if !found {
		log.WithFields(map[string]interface{}{"key": key}).Debug("Item not in cache, getting from source")
		var err error
		detail, err = fn(key)
		if err != nil {
			log.WithFields(map[string]interface{}{"key": key}).WithError(err).Debug("Error reading from source, returning error")
			return nil, errors.Join(errors.New("error reading: "+key), err)
		}
		if detail != nil && !reflect.ValueOf(detail).IsNil() {
			log.WithFields(map[string]interface{}{"key": key, "item": detail}).Debug("Item read from source, caching")
			c[key] = detail
		}
		return detail, nil
	}
	log.WithFields(map[string]interface{}{"key": key}).Debug("Item found in cache")
	return detail, nil
}
