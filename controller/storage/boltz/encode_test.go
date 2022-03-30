package boltz

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"
)

func TestEncodeDecodeStringSlice(t *testing.T) {
	req := require.New(t)
	for i := 0; i < 100; i++ {
		size := rand.Intn(10) + 1

		var ids []string
		for j := 0; j < size; j++ {
			ids = append(ids, uuid.New().String())
		}

		encoded, err := EncodeStringSlice(ids)
		req.NoError(err)

		decodedIds, err := DecodeStringSlice(encoded)
		req.NoError(err)
		req.Equal(ids, decodedIds)
	}
}
