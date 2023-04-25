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
	"context"
	"fmt"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"os"
	"testing"
)

type dbTest struct {
	errorz.ErrorHolderImpl
	*require.Assertions
	dbFile *os.File
	db     *bbolt.DB
}

func (test *dbTest) init() {
	var err error
	test.dbFile, err = os.CreateTemp("", "query-bolt-test-db")
	test.NoError(err)
	test.NoError(test.dbFile.Close())
	test.db, err = bbolt.Open(test.dbFile.Name(), 0, bbolt.DefaultOptions)
	test.NoError(err)
}

func (test *dbTest) cleanup() {
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

func (test *dbTest) switchTestContext(t *testing.T) {
	test.Assertions = require.New(t)
}

func newTestMutateContext(tx *bbolt.Tx) MutateContext {
	return NewTxMutateContext(context.Background(), tx)
}
