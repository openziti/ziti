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
	"github.com/golang/protobuf/proto"
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/sirupsen/logrus"
	"time"
)

type Scanner struct {
	ctrl        channel2.Channel
	sessions    *sessionTable
	interval    time.Duration
	timeout     time.Duration
	closeNotify <-chan struct{}
}

func NewScanner(options *Options, closeNotify <-chan struct{}) *Scanner {
	s := &Scanner{
		interval:    options.IdleTxInterval,
		timeout:     options.IdleSessionTimeout,
		closeNotify: closeNotify,
	}
	if s.interval > 0 {
		go s.run()
	} else {
		logrus.Warnf("scanner disabled")
	}
	return s
}

func (self *Scanner) SetCtrl(ch channel2.Channel) {
	self.ctrl = ch
}

func (self *Scanner) setSessionTable(sessions *sessionTable) {
	self.sessions = sessions
}

func (self *Scanner) run() {
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

func (self *Scanner) scan() {
	sessions := self.sessions.sessions.Items()
	logrus.Infof("scanning [%d] sessions", len(sessions))

	var idleSessionIds []string
	for sessionId, ft := range sessions {
		if time.Since(ft.(*forwardTable).last) > self.timeout {
			idleSessionIds = append(idleSessionIds, sessionId)
			logrus.Warnf("[s/%s] idle after [%s]", sessionId, self.timeout)
		}
	}

	if len(idleSessionIds) > 0 {
		logrus.Infof("found [%d] idle sessions, confirming with controller", len(idleSessionIds))

		if self.ctrl != nil {
			confirm := &ctrl_pb.SessionConfirmation{}
			for _, idleSessionId := range idleSessionIds {
				confirm.SessionIds = append(confirm.SessionIds, idleSessionId)
			}
			body, err := proto.Marshal(confirm)
			if err == nil {
				msg := channel2.NewMessage(ctrl_msg.SessionConfirmationType, body)
				if err := self.ctrl.Send(msg); err == nil {
					logrus.Warnf("sent confirmation for [%d] sessions", len(idleSessionIds))
				} else {
					logrus.Errorf("error sending confirmation request (%v)", err)
				}
			}
		} else {
			logrus.Errorf("no ctrl channel, cannot request session confirmations")
		}
	}
}
