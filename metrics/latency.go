/*
	Copyright 2019 Netfoundry, Inc.

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

package metrics

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/util/info"
	"time"
)

// send regular latency probes
//
func ProbeLatency(ch channel2.Channel, histogram Histogram, interval time.Duration) {
	log := pfxlog.ContextLogger(ch.Label())
	log.Info("started")
	defer log.Warn("exited")
	defer func() {
		histogram.Dispose()
	}()

	lastSend := info.NowInMilliseconds()
	for {
		time.Sleep(interval)
		if ch.IsClosed() {
			return
		}

		now := info.NowInMilliseconds()
		if now-lastSend > 10000 {
			lastSend = now
			request := channel2.NewMessage(channel2.ContentTypeLatencyType, nil)
			request.PutUint64Header(latencyProbeTime, uint64(time.Now().UnixNano()))
			waitCh, err := ch.SendAndWaitWithPriority(request, channel2.High)
			if err != nil {
				log.Errorf("unexpected error sending latency probe (%s)", err)
				continue
			}

			select {
			case response := <-waitCh:
				if response == nil {
					log.Error("wait channel closed")
					return
				}
				if response.ContentType == channel2.ContentTypeResultType {
					result := channel2.UnmarshalResult(response)
					if result.Success {
						if sentTime, ok := response.GetUint64Header(latencyProbeTime); ok {
							latency := time.Now().UnixNano() - int64(sentTime)
							histogram.Update(latency)
						} else {
							log.Error("no send time")
						}
					} else {
						log.Error("failed latency response")
					}
				} else {
					log.Errorf("unexpected latency response [%d]", response.ContentType)
				}
			case <-time.After(time.Second * 5):
				log.Error("latency timeout")
			}
		}
	}
}
