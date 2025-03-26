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

package xgress

import (
	"encoding/json"
	"github.com/pkg/errors"
	"time"
)

// Options contains common Xgress configuration options
type Options struct {
	Mtu         int32
	RandomDrops bool
	Drop1InN    int32
	TxQueueSize int32

	TxPortalStartSize      uint32
	TxPortalMaxSize        uint32
	TxPortalMinSize        uint32
	TxPortalIncreaseThresh uint32
	TxPortalIncreaseScale  float64
	TxPortalRetxThresh     uint32
	TxPortalRetxScale      float64
	TxPortalDupAckThresh   uint32
	TxPortalDupAckScale    float64

	RxBufferSize uint32
	RetxStartMs  uint32
	RetxScale    float64
	RetxAddMs    uint32

	MaxCloseWait        time.Duration
	GetCircuitTimeout   time.Duration
	CircuitStartTimeout time.Duration
	ConnectTimeout      time.Duration
}

func LoadOptions(data OptionsData) (*Options, error) {
	options := DefaultOptions()

	if value, found := data["options"]; found {
		data = value.(map[interface{}]interface{})

		if value, found := data["mtu"]; found {
			options.Mtu = int32(value.(int))
		}
		if value, found := data["randomDrops"]; found {
			options.RandomDrops = value.(bool)
		}
		if value, found := data["drop1InN"]; found {
			options.Drop1InN = int32(value.(int))
		}
		if value, found := data["txQueueSize"]; found {
			options.TxQueueSize = int32(value.(int))
		}

		if value, found := data["txPortalStartSize"]; found {
			options.TxPortalStartSize = uint32(value.(int))
		}
		if value, found := data["txPortalMinSize"]; found {
			options.TxPortalMinSize = uint32(value.(int))
		}
		if value, found := data["txPortalMaxSize"]; found {
			options.TxPortalMaxSize = uint32(value.(int))
		}
		if value, found := data["txPortalIncreaseThresh"]; found {
			options.TxPortalIncreaseThresh = uint32(value.(int))
		}
		if value, found := data["txPortalIncreaseScale"]; found {
			options.TxPortalIncreaseScale = value.(float64)
		}
		if value, found := data["txPortalRetxThresh"]; found {
			options.TxPortalRetxThresh = uint32(value.(int))
		}
		if value, found := data["txPortalRetxScale"]; found {
			options.TxPortalRetxScale = value.(float64)
		}
		if value, found := data["txPortalDupAckThresh"]; found {
			options.TxPortalDupAckThresh = uint32(value.(int))
		}
		if value, found := data["txPortalDupAckScale"]; found {
			options.TxPortalDupAckScale = value.(float64)
		}

		if value, found := data["rxBufferSize"]; found {
			options.RxBufferSize = uint32(value.(int))
		}
		if value, found := data["retxStartMs"]; found {
			options.RetxStartMs = uint32(value.(int))
		}
		if value, found := data["retxScale"]; found {
			options.RetxScale = value.(float64)
		}
		if value, found := data["retxAddMs"]; found {
			options.RetxAddMs = uint32(value.(int))
		}

		if value, found := data["maxCloseWaitMs"]; found {
			options.MaxCloseWait = time.Duration(value.(int)) * time.Millisecond
		}

		if value, found := data["getCircuitTimeout"]; found {
			getCircuitTimeout, err := time.ParseDuration(value.(string))
			if err != nil {
				return nil, errors.Wrap(err, "invalid 'getCircuitTimeout' value")
			}
			options.GetCircuitTimeout = getCircuitTimeout
		}

		if value, found := data["circuitStartTimeout"]; found {
			circuitStartTimeout, err := time.ParseDuration(value.(string))
			if err != nil {
				return nil, errors.Wrap(err, "invalid 'circuitStartTimeout' value")
			}
			options.CircuitStartTimeout = circuitStartTimeout
		}

		if value, found := data["connectTimeout"]; found {
			connectTimeout, err := time.ParseDuration(value.(string))
			if err != nil {
				return nil, errors.Wrap(err, "invalid 'connectTimeout' value")
			}
			options.ConnectTimeout = connectTimeout
		}
	}

	return options, nil
}

func DefaultOptions() *Options {
	return &Options{
		Mtu:                    0,
		RandomDrops:            false,
		Drop1InN:               100,
		TxQueueSize:            1,
		TxPortalStartSize:      4 * 1024 * 1024,
		TxPortalMinSize:        16 * 1024,
		TxPortalMaxSize:        4 * 1024 * 1024,
		TxPortalIncreaseThresh: 28,
		TxPortalIncreaseScale:  1.0,
		TxPortalRetxThresh:     64,
		TxPortalRetxScale:      0.75,
		TxPortalDupAckThresh:   64,
		TxPortalDupAckScale:    0.9,
		RxBufferSize:           4 * 1024 * 1024,
		RetxStartMs:            200,
		RetxScale:              1.5,
		RetxAddMs:              0,
		MaxCloseWait:           30 * time.Second,
		GetCircuitTimeout:      30 * time.Second,
		CircuitStartTimeout:    3 * time.Minute,
		ConnectTimeout:         0, // operating system default
	}
}

func (options Options) String() string {
	data, err := json.Marshal(options)
	if err != nil {
		return err.Error()
	}
	return string(data)
}
