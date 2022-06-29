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

package xctrl_example

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/xctrl"
	"github.com/openziti/storage/boltz"
	"github.com/sirupsen/logrus"
	"time"
)

type example struct {
	count   int32
	delay   int
	enabled bool
}

func NewExample() xctrl.Xctrl {
	return &example{delay: 1, enabled: false}
}

func (example *example) LoadConfig(cfgmap map[interface{}]interface{}) error {
	if value, found := cfgmap["example"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["delay"]; found {
				delay, ok := value.(int)
				if !ok {
					return errors.New("expected int value for [example/delay]")
				}
				example.delay = delay
			}
		}
		example.enabled = true
	}
	return nil
}

func (example *example) Enabled() bool {
	return example.enabled
}

func (example *example) BindChannel(binding channel.Binding) error {
	binding.AddTypedReceiveHandler(newReceiveHandler())
	return nil
}

func (example *example) Run(ctrl channel.Channel, _ boltz.Db, done chan struct{}) error {
	go func() {
		for {
			select {
			case <-done:
				pfxlog.Logger().Infof("exiting")
				return

			case <-time.After(time.Duration(example.delay) * time.Second):
				body := new(bytes.Buffer)
				if err := binary.Write(body, binary.LittleEndian, example.count); err != nil {
					pfxlog.Logger().Errorf("unexpected error marshaling (%s)", err)
				}
				msg := channel.NewMessage(contentType, body.Bytes())
				if err := ctrl.Send(msg); err != nil {
					pfxlog.Logger().Errorf("unexpected error sending (%s)", err)
				}
				pfxlog.Logger().Infof("sent [%d]", example.count)
				example.count++
			}
		}
	}()
	return nil
}

func (example *example) NotifyOfReconnect() {
	logrus.Info("control channel reconnected")
}

func (example *example) GetTraceDecoders() []channel.TraceMessageDecoder {
	return nil
}

const contentType = 99999
