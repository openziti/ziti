package boltz

import (
	"fmt"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"io/ioutil"
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
	test.dbFile, err = ioutil.TempFile("", "query-bolt-test-db")
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
