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

package boltz

import (
	"encoding/json"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/openziti/foundation/util/errorz"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"io/ioutil"
	"os"
	"testing"
)

type bucketTest struct {
	errorz.ErrorHolderImpl
	*require.Assertions
	dbFile *os.File
	db     *bbolt.DB
}

func (test *bucketTest) init(constaint bool) {
	var err error
	test.dbFile, err = ioutil.TempFile("", "typed-bucket-test-db")
	test.NoError(err)
	test.NoError(test.dbFile.Close())
	test.db, err = bbolt.Open(test.dbFile.Name(), 0, bbolt.DefaultOptions)
	test.NoError(err)
}

func (test *bucketTest) cleanup() {
	if test.db != nil {
		if err := test.db.Close(); err != nil {
			fmt.Printf("error closing bolt db: %v", err)
		}
	}

	if test.dbFile != nil {
		if err := os.Remove(test.dbFile.Name()); err != nil {
			fmt.Printf("error deleting bolt db file: %v", err)
		}
	}
}

func TestTypedBuckets(t *testing.T) {
	test := &bucketTest{
		Assertions: require.New(t),
	}
	test.init(false)
	defer test.cleanup()

	t.Run("test maps", test.testMaps)
}

func (test *bucketTest) testMaps(t *testing.T) {
	test.Assertions = require.New(t)

	testMapSource := `
		{
            "boolArr" : [true, false, false, true],
            "numArr" : [1, 3, 4],
            "strArr" : ["hello", "world", "how", "are", "you?"],
			"mapArr" : [
				{
					"foo" : "bar"
				},
				{
					"bar" : "foo"
				}
			],
			"arrArr" : [
				[1 ,2 ,3, 6],
				["bing", "bang"]
			],
 			"nested": {
				"hello"    : "hi",
				"fromage?" : "that's cheese",
				"count"    :  1000.32,
				"vals" : [1, 5, 7],
				"how": {
					"nested": {
						"can":  "it be?",
						"beep": 2,
						"bop":  false
					}
				}
			}
        }`

	testMap := map[string]interface{}{}
	err := json.Unmarshal([]byte(testMapSource), &testMap)
	test.NoError(err)

	id := uuid.New().String()
	err = test.db.Update(func(tx *bbolt.Tx) error {
		basepath := GetOrCreatePath(tx, id)
		basepath.PutMap("test-map", testMap, nil, true)
		return basepath.GetError()
	})

	var testMapRead map[string]interface{}

	err = test.db.Update(func(tx *bbolt.Tx) error {
		basepath := GetOrCreatePath(tx, id)
		testMapRead = basepath.GetMap("test-map")
		return basepath.GetError()
	})
	test.NoError(err)

	test.True(cmp.Equal(testMap, testMapRead), cmp.Diff(testMap, testMapRead))
}
