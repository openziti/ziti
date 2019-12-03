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

package edge

import (
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/util/concurrenz"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"io"
	"time"
)

type MsgSink interface {
	io.Closer
	Id() uint32
	Accept(event *MsgEvent)
}

func NewMsgMux() *MsgMux {
	mux := &MsgMux{
		eventC:  make(chan MuxEvent),
		chanMap: make(map[uint32]MsgSink),
	}

	mux.running.Set(true)
	go mux.handleEvents()
	return mux
}

type MsgMux struct {
	closed  concurrenz.AtomicBoolean
	running concurrenz.AtomicBoolean
	eventC  chan MuxEvent
	chanMap map[uint32]MsgSink
}

func (mux *MsgMux) ContentType() int32 {
	return ContentTypeData
}

func (mux *MsgMux) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	if event, err := UnmarshalMsgEvent(msg); err != nil {
		pfxlog.Logger().WithError(err).Errorf("error unmarshaling edge message headers. content type: %v", msg.ContentType)
	} else {
		mux.eventC <- event
	}
}

func (mux *MsgMux) AddMsgSink(sink MsgSink) error {
	if !mux.closed.Get() {
		event := &muxAddSinkEvent{sink: sink, doneC: make(chan error)}
		mux.eventC <- event
		err, ok := <-event.doneC // wait for event to be done processing
		if ok && err != nil {
			return err
		}
		pfxlog.Logger().WithField("connId", sink.Id()).Debug("added to msg mux")
	}
	return nil
}

func (mux *MsgMux) RemoveMsgSink(sink MsgSink) {
	mux.RemoveMsgSinkById(sink.Id())
}

func (mux *MsgMux) RemoveMsgSinkById(sinkId uint32) (MsgSink, bool) {
	if !mux.closed.Get() {
		event := &muxRemoveSinkEvent{
			sinkId: sinkId,
			doneC:  make(chan MsgSink),
		}
		mux.eventC <- event
		sink, ok := <-event.doneC
		if ok && sink != nil {
			pfxlog.Logger().WithField("connId", sinkId).Debug("removed from msg mux")
			return sink, true
		}
	}
	pfxlog.Logger().WithField("connId", sinkId).Debug("sink not found in msg mux")
	return nil, false
}

func (mux *MsgMux) Close() {
	if !mux.closed.Get() {
		mux.eventC <- &muxCloseEvent{}
	}
}

func (mux *MsgMux) Event(event MuxEvent) {
	if !mux.closed.Get() {
		mux.eventC <- event
	}
}

func (mux *MsgMux) IsClosed() bool {
	return mux.closed.Get()
}

func (mux *MsgMux) HandleClose(ch channel2.Channel) {
	mux.Close()
}

func (mux *MsgMux) handleEvents() {
	defer mux.running.Set(false)
	for event := range mux.eventC {
		event.Handle(mux)
		if mux.closed.GetUnsafe() {
			return
		}
	}
}

func (mux *MsgMux) ExecuteClose() {
	mux.closed.Set(true)
	for _, val := range mux.chanMap {
		if err := val.Close(); err != nil {
			pfxlog.Logger().
				WithField("sinkId", val.Id()).
				WithError(err).
				Error("error while closing message sink")
		}
	}

	// make sure that anything trying to deliver events is freed
	for {
		select {
		case <-mux.eventC: // drop event
		case <-time.After(time.Millisecond * 100):
			close(mux.eventC)
			return
		}
	}
}

type MuxEvent interface {
	Handle(mux *MsgMux)
}

// muxAddSinkEvent handles adding a new message sink to the mux
type muxAddSinkEvent struct {
	sink  MsgSink
	doneC chan error
}

func (event *muxAddSinkEvent) Handle(mux *MsgMux) {
	defer close(event.doneC)
	if _, found := mux.chanMap[event.sink.Id()]; found {
		event.doneC <- fmt.Errorf("message sink with id %v already exists", event.sink.Id())
	} else {
		mux.chanMap[event.sink.Id()] = event.sink
		pfxlog.Logger().
			WithField("connId", event.sink.Id()).
			Debugf("Added sink to mux. Current sink count: %v", len(mux.chanMap))
	}
}

// muxRemoveSinkEvent handles removing a closed message sink from the mux
type muxRemoveSinkEvent struct {
	sinkId uint32
	doneC  chan MsgSink
}

func (event *muxRemoveSinkEvent) Handle(mux *MsgMux) {
	defer close(event.doneC)
	sink, found := mux.chanMap[event.sinkId]
	delete(mux.chanMap, event.sinkId)
	if found {
		event.doneC <- sink
	}
	pfxlog.Logger().
		WithField("connId", event.sinkId).
		Debugf("Removed sink from mux. Current sink count: %v", len(mux.chanMap))
}

func (event *MsgEvent) Handle(mux *MsgMux) {
	logger := pfxlog.Logger().
		WithField("seq", event.Seq).
		WithField("connId", event.ConnId)

	logger.Debugf("handling received %v", ContentTypeNames[event.Msg.ContentType])

	if sink, found := mux.chanMap[event.ConnId]; found {
		sink.Accept(event)
	} else {
		logger.Debug("msg received for unknown edge channel id")
	}
}

// muxCloseEvent handles closing the message multiplexer and all associated sinks
type muxCloseEvent struct{}

func (event *muxCloseEvent) Handle(mux *MsgMux) {
	mux.ExecuteClose()
}
