package loop4

import (
	"bytes"
	"crypto/rand"
	"crypto/sha512"
	"github.com/google/go-cmp/cmp"
	"github.com/openziti/metrics"
	loopPb "github.com/openziti/ziti/zititest/ziti-traffic-test/subcmd/loop4/pb"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

type testPeer struct {
	bytes.Buffer
}

func (t *testPeer) Close() error {
	return nil
}

func Test_MessageSerDeser(t *testing.T) {
	req := require.New(t)
	data := make([]byte, 4192)
	_, err := rand.Read(data)
	req.NoError(err)

	hash := sha512.Sum512(data)

	block := &RandHashedBlock{
		Type:     BlockTypePlain,
		Sequence: 10,
		Hash:     hash[:],
		Data:     data,
	}

	testBuf := &testPeer{}

	metricsRegistry := metrics.NewRegistry("test", nil)

	p, err := newProtocol(testBuf, "test", metricsRegistry)
	req.NoError(err)

	p.test = &loopPb.Test{
		Name: "test",
	}

	req.NoError(block.Tx(p))

	readBlock := &RandHashedBlock{}
	req.NoError(readBlock.Rx(p))

	req.True(reflect.DeepEqual(block, readBlock), cmp.Diff(block, readBlock))

	data = make([]byte, 4192)
	_, err = rand.Read(data)
	req.NoError(err)

	hash = sha512.Sum512(data)

	block = &RandHashedBlock{
		Type:     BlockTypeLatencyRequest,
		Sequence: 10,
		Hash:     hash[:],
		Data:     data,
	}

	req.NoError(block.Tx(p))

	readBlock = &RandHashedBlock{}
	req.NoError(readBlock.Rx(p))

	req.Equal("", cmp.Diff(block, readBlock))
}
