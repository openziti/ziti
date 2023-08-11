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

package loop2

import (
	"github.com/openziti/ziti/zititest/ziti-fabric-test/subcmd/loop2/pb"
	"github.com/openziti/foundation/v2/info"
	"crypto/sha512"
	"github.com/michaelquigley/pfxlog"
	"math/rand"
)

type generator struct {
	count   int
	minSize int
	maxSize int
	blocks  chan *loop2_pb.Block
	pool    [][]byte
}

func newGenerator(count, minSize, maxSize int) *generator {
	g := &generator{
		count:   count,
		minSize: minSize,
		maxSize: maxSize,
		blocks:  make(chan *loop2_pb.Block),
		pool:    newPool(),
	}
	return g
}

func (g *generator) run() {
	log := pfxlog.Logger()
	log.Debug("started")
	defer log.Debug("complete")

	for i := 0; i < g.count; i++ {
		size := g.minSize
		distance := g.maxSize - g.minSize
		if distance > 0 {
			size += rand.Intn(distance)
		}
		data := make([]byte, size)
		for idx := 0; idx < size; {
			bucket := g.pool[rand.Intn(len(g.pool))]
			for i := 0; i < len(bucket) && idx < size; i++ {
				data[idx] = bucket[i]
				idx++
			}
		}
		hash := sha512.Sum512(data)
		g.blocks <- &loop2_pb.Block{
			Sequence: int32(i),
			Data:     data,
			Hash:     hash[:],
		}
	}
}

func newPool() [][]byte {
	log := pfxlog.Logger()
	start := info.NowInMilliseconds()
	log.Debug("building")
	defer func() {
		log.Debugf("complete (%d)", info.NowInMilliseconds()-start)
	}()

	buckets := 64
	pool := make([][]byte, buckets)
	for i := 0; i < buckets; i++ {
		length := 4096
		pool[i] = make([]byte, 0)
		for j := 0; j < length; j++ {
			pool[i] = append(pool[i], byte(rand.Intn(255)))
		}
	}
	return pool
}
