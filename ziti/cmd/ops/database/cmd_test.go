package database

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func createBoltDbWithEdgeVersion(t *testing.T, path string, versionValue string) {
	t.Helper()

	opts := *bbolt.DefaultOptions
	opts.ReadOnly = false

	db, err := bbolt.Open(path, 0600, &opts)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	err = db.Update(func(tx *bbolt.Tx) error {
		root, err := tx.CreateBucketIfNotExists([]byte(RootBucketName))
		if err != nil {
			return err
		}
		versionBucket, err := root.CreateBucketIfNotExists([]byte("version"))
		if err != nil {
			return err
		}
		return versionBucket.Put([]byte("edge"), []byte(versionValue))
	})
	require.NoError(t, err)
}

func gzipFile(t *testing.T, srcPath string, dstPath string) {
	t.Helper()

	data, err := os.ReadFile(srcPath)
	require.NoError(t, err)

	f, err := os.Create(dstPath)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	_, err = gw.Write(data)
	require.NoError(t, err)
	require.NoError(t, gw.Close())
}

func runDatastoreVersionCmd(t *testing.T, arg string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cmd := NewDatastoreVersionCmd(&out)
	cmd.SetArgs([]string{arg})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

func TestDatastoreVersionCmd_BoltDbFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "ctrl.db")
	createBoltDbWithEdgeVersion(t, path, "43")

	out, err := runDatastoreVersionCmd(t, path)
	require.NoError(t, err)
	require.Equal(t, "43\n", out)
}

func TestDatastoreVersionCmd_RaftDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	raftDir := filepath.Join(tmpDir, "raft")
	require.NoError(t, os.MkdirAll(raftDir, 0755))
	path := filepath.Join(raftDir, "ctrl-ha.db")
	createBoltDbWithEdgeVersion(t, path, "44")

	out, err := runDatastoreVersionCmd(t, raftDir)
	require.NoError(t, err)
	require.Equal(t, "44\n", out)
}

func TestDatastoreVersionCmd_GzipSnapshotFile(t *testing.T) {
	tmpDir := t.TempDir()
	boltPath := filepath.Join(tmpDir, "snapshot.db")
	createBoltDbWithEdgeVersion(t, boltPath, "41")

	gzPath := filepath.Join(tmpDir, "snapshot.db.gz")
	gzipFile(t, boltPath, gzPath)

	out, err := runDatastoreVersionCmd(t, gzPath)
	require.NoError(t, err)
	require.Equal(t, "41\n", out)
}

func TestDatastoreVersionCmd_InvalidEdgeVersionValue(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "ctrl.db")
	createBoltDbWithEdgeVersion(t, path, "not-an-int")

	_, err := runDatastoreVersionCmd(t, path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid edge version value")
}

func TestDatastoreVersionCmd_MissingRootBucket(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "ctrl.db")

	opts := *bbolt.DefaultOptions
	opts.ReadOnly = false
	db, err := bbolt.Open(path, 0600, &opts)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = runDatastoreVersionCmd(t, path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "root")
	require.Contains(t, err.Error(), "bucket not found")
}

func TestDatastoreVersionCmd_MissingVersionBucket(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "ctrl.db")

	opts := *bbolt.DefaultOptions
	db, err := bbolt.Open(path, 0600, &opts)
	require.NoError(t, err)
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(RootBucketName))
		return err
	}))
	require.NoError(t, db.Close())

	_, err = runDatastoreVersionCmd(t, path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "'version' bucket not found")
}

func TestDatastoreVersionCmd_MissingEdgeVersionKey(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "ctrl.db")

	opts := *bbolt.DefaultOptions
	db, err := bbolt.Open(path, 0600, &opts)
	require.NoError(t, err)
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		root, err := tx.CreateBucketIfNotExists([]byte(RootBucketName))
		if err != nil {
			return err
		}
		_, err = root.CreateBucketIfNotExists([]byte("version"))
		return err
	}))
	require.NoError(t, db.Close())

	_, err = runDatastoreVersionCmd(t, path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "'edge' version not found")
}
