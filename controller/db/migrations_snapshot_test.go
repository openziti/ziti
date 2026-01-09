package db

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/command"
	"github.com/stretchr/testify/require"
)

func TestRunMigrations_DatastoreVersionTooHigh_DoesNotSnapshotAndErrors(t *testing.T) {
	req := require.New(t)

	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	dbPath := ctx.BaseTestContext.GetDbFile().Name()

	// Close the db/stores created by the test context and create a clean DB at the same path
	req.NoError(ctx.GetDb().Close())

	zdb, err := Open(dbPath)
	req.NoError(err)
	defer func() {
		_ = zdb.Close()
	}()

	stores, err := InitStores(zdb, command.NoOpRateLimiter{}, nil)
	req.NoError(err)

	tooHigh := CurrentDbVersion + 10
	// Set edge component version higher than what this controller supports
	req.NoError(zdb.Update(nil, func(mctx boltz.MutateContext) error {
		versionsBucket := boltz.GetOrCreatePath(mctx.Tx(), RootBucket, "versions")
		versionsBucket.SetInt64("edge", int64(tooHigh), nil)
		return versionsBucket.GetError()
	}))

	// Ensure no snapshots exist before running
	before, err := filepath.Glob(dbPath + "-*")
	req.NoError(err)
	req.Len(before, 0)

	err = RunMigrations(zdb, stores, nil)
	req.Error(err)
	msg := strings.ToLower(err.Error())
	req.Contains(msg, "edge datastore version is too high")
	req.Contains(msg, "supports <=")
	req.Contains(msg, "upgrade")
	req.Contains(msg, "snapshot")
	req.Contains(msg, "restoring")

	after, err := filepath.Glob(dbPath + "-*")
	req.NoError(err)
	req.Len(after, 0)

}

func TestRunMigrations_OlderDatastore_TakesSnapshotWithVersionInFilename_ThenMigrates(t *testing.T) {
	req := require.New(t)

	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	dbPath := ctx.BaseTestContext.GetDbFile().Name()

	// Close the db/stores created by the test context and create a clean DB at the same path
	req.NoError(ctx.GetDb().Close())

	zdb, err := Open(dbPath)
	req.NoError(err)
	defer func() {
		_ = zdb.Close()
	}()

	stores, err := InitStores(zdb, command.NoOpRateLimiter{}, nil)
	req.NoError(err)

	mm := boltz.NewMigratorManager(zdb)
	oldVersion := CurrentDbVersion - 1

	req.NoError(zdb.Update(nil, func(mctx boltz.MutateContext) error {
		versionsBucket := boltz.GetOrCreatePath(mctx.Tx(), RootBucket, "versions")
		versionsBucket.SetInt64("edge", int64(oldVersion), nil)
		return versionsBucket.GetError()
	}))

	before, err := filepath.Glob(dbPath + "-*")
	req.NoError(err)
	req.Len(before, 0)

	err = RunMigrations(zdb, stores, nil)
	req.NoError(err)

	current, err := mm.GetComponentVersion("edge")
	req.NoError(err)
	req.Equal(CurrentDbVersion, current)

	after, err := filepath.Glob(dbPath + "-*")
	req.NoError(err)
	req.NotEmpty(after)

	found := false
	for _, p := range after {
		// boltz default snapshot name is: <dbpath>-YYYYMMDD-HHMMSS
		// our convention appends "-edge-v<oldVersion>"
		if strings.Contains(filepath.Base(p), "-edge-v"+strconv.Itoa(oldVersion)) {
			found = true
			break
		}
	}
	req.True(found, "expected snapshot filename to include datastore version suffix")

	// cleanup snapshots created during test to avoid leaving temp files behind in some environments
	for _, p := range after {
		_ = os.Remove(p)
	}
}
