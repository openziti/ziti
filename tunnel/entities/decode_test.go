package entities

import (
	"bytes"
	"encoding/json"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_LoadHttpCheck(t *testing.T) {
	req := require.New(t)

	var test = `
        {
			"protocol" : "tcp",
			"hostname" : "localhost",
			"port" : 8171,
			"httpChecks" : [
				{
					"interval" : "1s",
					"timeout" : "500ms",
					"url" : "http://localhost:5432"
				}
			]
		}
`

	buf := bytes.NewBufferString(test)
	d := json.NewDecoder(buf)

	m := map[string]interface{}{}
	req.NoError(d.Decode(&m))

	config := &ServiceConfig{}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:     config,
		DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
	})
	req.NoError(err)
	req.NoError(decoder.Decode(m))
}
