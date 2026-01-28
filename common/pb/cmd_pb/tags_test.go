package cmd_pb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagEncodeDecode(t *testing.T) {
	tags := map[string]interface{}{}
	tags["str"] = "string"
	tags["str2"] = "another string"
	tags["blt"] = true
	tags["blf"] = false
	tags["nil"] = nil
	tags["fl"] = float64(123)

	encoded, err := EncodeTags(tags)
	assert.NoError(t, err)
	assert.Equal(t, len(tags), len(encoded))

	decoded := DecodeTags(encoded)
	assert.Equal(t, tags, decoded)
}
