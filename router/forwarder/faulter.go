/*
	Copyright NetFoundry, Inc.

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

package forwarder

import (
	cmap "github.com/orcaman/concurrent-map"
	"github.com/sirupsen/logrus"
	"time"
)

type faulter struct {
	interval time.Duration
	sessionIds cmap.ConcurrentMap // map[sessionId]struct{}
}

func newFaulter(interval time.Duration) *faulter {
	f := &faulter{interval: interval, sessionIds: cmap.New()}
	go f.run()
	return f
}

func (self *faulter) report(sessionId string) {
	self.sessionIds.Set(sessionId, struct{}{})
}

func (self *faulter) run() {
	logrus.Infof("started")
	defer logrus.Errorf("exited")

	for {
		time.Sleep(self.interval)
		logrus.Infof("transmitting faults")
	}
}



