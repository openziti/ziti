package posture

import (
	"sync"
	"testing"

	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/stretchr/testify/require"
)

func procResponse(path, hash string) *edge_client_pb.PostureResponse {
	return processListResponse(&edge_client_pb.PostureResponse_Process{
		Path: path, IsRunning: true, Hash: hash,
	})
}

func procBatch(hash string) []*edge_client_pb.PostureResponse {
	return []*edge_client_pb.PostureResponse{
		procResponse("/bin/a", hash), procResponse("/bin/b", hash), procResponse("/bin/c", hash),
	}
}

// Test_ApplyBatch_ReaderNeverSeesMixedBatch covers the interleaving that revokes access from a
// client that is in fact compliant. The Go SDK reports one process-list entry per watched process,
// so a batch that updates several processes applies them one at a time. A reader must observe the
// state before the batch or after all of it: a mix of old and new entries can fail an AllOf
// process check that both the previous and the resulting state pass.
func Test_ApplyBatch_ReaderNeverSeesMixedBatch(t *testing.T) {
	req := require.New(t)

	for range 500 {
		instance := newInstance()
		req.True(instance.ApplyBatch(procBatch("v1"), nil))

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			instance.ApplyBatch(procBatch("v2"), nil)
		}()

		var hashes []string
		go func() {
			defer wg.Done()
			for _, proc := range instance.Snapshot().ProcessList.GetProcesses() {
				hashes = append(hashes, proc.Hash)
			}
		}()

		wg.Wait()

		req.Len(hashes, 3)
		for _, hash := range hashes {
			req.Equal(hashes[0], hash,
				"a reader must see the batch fully applied or not at all, got a mix: %v", hashes)
		}
	}
}

// Test_ApplyBatch_ReaderNeverSeesPartialGrowth is the same requirement for a batch that adds
// processes rather than updating them: the set is empty or complete, never half built.
func Test_ApplyBatch_ReaderNeverSeesPartialGrowth(t *testing.T) {
	req := require.New(t)

	for range 500 {
		instance := newInstance()

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			instance.ApplyBatch(procBatch("v1"), nil)
		}()

		var observed int
		go func() {
			defer wg.Done()
			observed = len(instance.Snapshot().ProcessList.GetProcesses())
		}()

		wg.Wait()
		req.Contains([]int{0, 3}, observed, "a reader must not observe a partially applied batch")
	}
}

func Test_ApplyBatch_AppliesEveryResponse(t *testing.T) {
	req := require.New(t)
	instance := newInstance()

	req.True(instance.ApplyBatch([]*edge_client_pb.PostureResponse{
		{Type: &edge_client_pb.PostureResponse_Domain_{
			Domain: &edge_client_pb.PostureResponse_Domain{Name: "example.com"}}},
		macResponse("00:11:22:33:44:55"),
		procResponse("/bin/a", "abc"),
	}, nil))

	data := instance.Snapshot()
	req.Equal("example.com", data.Domain.GetName())
	req.Equal([]string{"001122334455"}, data.Macs.GetAddresses())
	req.Len(data.ProcessList.GetProcesses(), 1)
}

func Test_ApplyBatch_UnchangedBatchReportsNoUpdate(t *testing.T) {
	req := require.New(t)
	instance := newInstance()

	batch := []*edge_client_pb.PostureResponse{procResponse("/bin/a", "abc"), procResponse("/bin/b", "abc")}
	req.True(instance.ApplyBatch(batch, nil))
	req.False(instance.ApplyBatch(batch, nil), "re-reporting the same batch is not a change")
}

func Test_ApplyBatch_EmptyBatchReportsNoUpdate(t *testing.T) {
	require.False(t, newInstance().ApplyBatch(nil, nil))
}
