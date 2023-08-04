package health

import (
	"bytes"
	"encoding/json"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func Test_LoadPingTest(t *testing.T) {
	req := require.New(t)

	var test = `
        {
            "type" : "port-check",
            "interval" : "10s",
            "address" : "localhost:5554",
			"actions": [
				{
					"action": "mark unhealthy",
					"consecutiveEvents": 10,
					"trigger": "fail"
				},
				{
					"action": "increase cost 100",
					"trigger": "fail"
				},
				{
					"action": "mark healthy",
					"duration": "1m",
					"trigger": "pass"
				},
				{
					"action": "decrease cost 5",
					"trigger": "pass"
				}
			]
        }`

	buf := bytes.NewBufferString(test)
	d := json.NewDecoder(buf)

	m := map[string]interface{}{}
	req.NoError(d.Decode(&m))

	pingCheck := &PortCheckDefinition{}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:     pingCheck,
		DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
	})
	req.NoError(err)
	req.NoError(decoder.Decode(m))

	req.Equal(pingCheck.Address, "localhost:5554")
	req.Equal(10*time.Second, pingCheck.Interval)
	req.NotNil(pingCheck.Actions)
	req.Equal(4, len(pingCheck.Actions))

	req.Equal("fail", pingCheck.Actions[0].Trigger)
	req.NotNil(pingCheck.Actions[0].ConsecutiveEvents)
	req.Equal(uint16(10), *pingCheck.Actions[0].ConsecutiveEvents)
	req.Nil(pingCheck.Actions[0].Duration)
	req.Equal("mark unhealthy", pingCheck.Actions[0].Action)

	req.Equal("fail", pingCheck.Actions[1].Trigger)
	req.Nil(pingCheck.Actions[1].ConsecutiveEvents)
	req.Nil(pingCheck.Actions[1].Duration)
	req.Equal("increase cost 100", pingCheck.Actions[1].Action)

	req.Equal("pass", pingCheck.Actions[2].Trigger)
	req.Nil(pingCheck.Actions[2].ConsecutiveEvents)
	req.NotNil(pingCheck.Actions[2].Duration)
	req.Equal(time.Minute, *pingCheck.Actions[2].Duration)
	req.Equal("mark healthy", pingCheck.Actions[2].Action)

	req.Equal("pass", pingCheck.Actions[3].Trigger)
	req.Nil(pingCheck.Actions[3].ConsecutiveEvents)
	req.Nil(pingCheck.Actions[3].Duration)
	req.Equal("decrease cost 5", pingCheck.Actions[3].Action)
}
