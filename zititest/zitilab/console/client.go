/*
	Copyright 2019 NetFoundry Inc.

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

package console

import (
	"github.com/sirupsen/logrus"
	"golang.org/x/net/websocket"
)

func NewClient(ws *websocket.Conn, server *Server) *Client {
	nextId++
	return &Client{
		id:     nextId,
		ws:     ws,
		server: server,
		ch:     make(chan *Message, chBufSize),
		doneCh: make(chan struct{}),
	}
}

func (client *Client) Conn() *websocket.Conn {
	return client.ws
}

func (client *Client) Write(msg *Message) {
	select {
	case client.ch <- msg:
	default:
	}
}

func (client *Client) Listen() {
	client.listenWrite()
}

func (client *Client) listenWrite() {
	for {
		select {
		case msg := <-client.ch:
			if err := websocket.JSON.Send(client.ws, msg); err != nil {
				logrus.Errorf("error sending to client [#%d] (%v)", client.id, err)
			}

		case <-client.doneCh:
			client.server.Del(client)
			close(client.doneCh)
			return
		}
	}
}

type Client struct {
	id     int
	ws     *websocket.Conn
	server *Server
	ch     chan *Message
	doneCh chan struct{}
}

var nextId int = 0

const chBufSize = 100
