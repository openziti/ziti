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
	"github.com/sirupsen/logrus"
	"time"
)

type scanner struct {
	sessions    *sessionTable
	interval    time.Duration
	timeout     time.Duration
	closeNotify <-chan struct{}
}

func newScanner(sessions *sessionTable, interval time.Duration, timeout time.Duration, closeNotify <-chan struct{}) *scanner {
	return &scanner{
		sessions:    sessions,
		interval:    interval,
		timeout:     timeout,
		closeNotify: closeNotify,
	}
}

func (self *scanner) run() {
	logrus.Info("started")
	defer logrus.Warn("exited")

	for {
		select {
		case <-time.After(self.interval):
			self.scan()

		case <-self.closeNotify:
			return
		}
	}
}

func (self *scanner) scan() {
	for sessionId, ft := range self.sessions.sessions.Items() {
		var idleSessionIds []string
		if time.Since(ft.(*forwardTable).last) > self.timeout {
			idleSessionIds = append(idleSessionIds, sessionId)
			logrus.Warnf("[s/%s] idle after [%s]", sessionId, self.timeout)
		}
	}
}
