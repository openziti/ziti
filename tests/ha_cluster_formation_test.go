//go:build apitests

package tests

import (
	"testing"
)

// Test_HaClusterFormation verifies the multi-controller test harness forms a full raft
// cluster: every member is a voter and every member is registered in the Controller store.
func Test_HaClusterFormation(t *testing.T) {
	ctx := NewTestContextWithConfigSet(t, Ha3)
	defer ctx.Teardown()
	ctx.StartHaCluster(Ha3DataDir)
}
