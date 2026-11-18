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

package posture

import (
	"sync"
	"testing"

	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/stretchr/testify/require"
)

func Test_ApplyBatch(t *testing.T) {
	// The Go SDK reports one process-list entry per watched process, so a batch that updates several
	// processes applies them one at a time. A reader that sees a mix of old and new entries can fail
	// an AllOf process check that both the previous and the resulting state pass, revoking access
	// from a client that is in fact compliant.
	t.Run("a reader never sees a batch of updates partly applied", func(t *testing.T) {
		req := require.New(t)

		for range 500 {
			// arrange
			instance := newInstance()
			req.True(instance.ApplyBatch([]*edge_client_pb.PostureResponse{
				procResponse("/bin/a", "v1"), procResponse("/bin/b", "v1"), procResponse("/bin/c", "v1"),
			}, nil))
			var hashes []string
			var wg sync.WaitGroup
			wg.Add(2)

			// act
			go applyBatch(instance, []*edge_client_pb.PostureResponse{
				procResponse("/bin/a", "v2"), procResponse("/bin/b", "v2"), procResponse("/bin/c", "v2"),
			}, &wg)
			go snapshotProcessHashes(instance, &hashes, &wg)
			wg.Wait()

			// assert
			req.Len(hashes, 3, "the snapshot must list every reported process")
			for _, hash := range hashes {
				req.Equal(hashes[0], hash,
					"a reader must see the batch fully applied or not at all, got a mix: %v", hashes)
			}
		}
	})

	t.Run("a reader never sees a batch of additions partly applied", func(t *testing.T) {
		req := require.New(t)

		for range 500 {
			// arrange
			instance := newInstance()
			var hashes []string
			var wg sync.WaitGroup
			wg.Add(2)

			// act
			go applyBatch(instance, []*edge_client_pb.PostureResponse{
				procResponse("/bin/a", "v1"), procResponse("/bin/b", "v1"), procResponse("/bin/c", "v1"),
			}, &wg)
			go snapshotProcessHashes(instance, &hashes, &wg)
			wg.Wait()

			// assert
			req.Contains([]int{0, 3}, len(hashes), "a reader must see none of the batch or all of it")
		}
	})

	t.Run("every response in a batch is applied", func(t *testing.T) {
		req := require.New(t)

		// arrange
		instance := newInstance()

		// act
		updated := instance.ApplyBatch([]*edge_client_pb.PostureResponse{
			{Type: &edge_client_pb.PostureResponse_Domain_{
				Domain: &edge_client_pb.PostureResponse_Domain{Name: "example.com"}}},
			macResponse("00:11:22:33:44:55"),
			procResponse("/bin/a", "abc"),
		}, nil)

		// assert
		req.True(updated, "a batch that changes state must report an update")
		data := instance.Snapshot()
		req.Equal("example.com", data.Domain.GetName())
		req.Equal([]string{"001122334455"}, data.Macs.GetAddresses(), "MAC addresses are stored normalized")
		req.Len(data.ProcessList.GetProcesses(), 1)
	})

	t.Run("re-applying an unchanged batch reports no update", func(t *testing.T) {
		req := require.New(t)

		// arrange
		instance := newInstance()
		batch := []*edge_client_pb.PostureResponse{procResponse("/bin/a", "abc"), procResponse("/bin/b", "abc")}
		req.True(instance.ApplyBatch(batch, nil))

		// act
		updated := instance.ApplyBatch(batch, nil)

		// assert
		req.False(updated, "re-reporting the same batch is not a change")
	})

	t.Run("an empty batch reports no update", func(t *testing.T) {
		req := require.New(t)

		// arrange
		instance := newInstance()

		// act
		updated := instance.ApplyBatch(nil, nil)

		// assert
		req.False(updated, "an empty batch changes nothing")
	})
}

// procResponse returns a process-list response reporting one running process.
func procResponse(path, hash string) *edge_client_pb.PostureResponse {
	return processListResponse(&edge_client_pb.PostureResponse_Process{
		Path: path, IsRunning: true, Hash: hash,
	})
}

// applyBatch applies batch to instance, then marks wg done.
func applyBatch(instance *Instance, batch []*edge_client_pb.PostureResponse, wg *sync.WaitGroup) {
	defer wg.Done()
	instance.ApplyBatch(batch, nil)
}

// snapshotProcessHashes stores in hashes the hash of each process in a snapshot of instance, then
// marks wg done.
func snapshotProcessHashes(instance *Instance, hashes *[]string, wg *sync.WaitGroup) {
	defer wg.Done()
	for _, proc := range instance.Snapshot().ProcessList.GetProcesses() {
		*hashes = append(*hashes, proc.Hash)
	}
}
