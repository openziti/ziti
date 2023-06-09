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
	"net/http"
)

func NewServer() *Server {
	return &Server{
		clients:   make(map[int]*Client),
		addCh:     make(chan *Client),
		delCh:     make(chan *Client),
		sendAllCh: make(chan *Message),
		doneCh:    make(chan struct{}),
		errCh:     make(chan error),
		routers:   make(map[string]bool),
		links:     make(map[string]*Link),
	}
}

func (server *Server) Add(c *Client) {
	server.addCh <- c
}

func (server *Server) Del(c *Client) {
	server.delCh <- c
}

func (server *Server) Done() {
	close(server.doneCh)
}

func (server *Server) SendAll(msg *Message) {
	server.sendAllCh <- msg
}

func (server *Server) sendAll(msg *Message) {
	for _, c := range server.clients {
		c.Write(msg)
	}
}

func (server *Server) Listen() {
	logrus.Infof("listening")

	onConnected := func(ws *websocket.Conn) {
		defer func() {
			if err := ws.Close(); err != nil {
				server.errCh <- err
			}
		}()

		client := NewClient(ws, server)
		server.Add(client)
		client.Listen()
	}
	http.Handle("/metrics", websocket.Handler(onConnected))

	for {
		select {
		case client := <-server.addCh:
			server.clients[client.id] = client
			server.sendNetwork(client)

		case client := <-server.delCh:
			delete(server.clients, client.id)

		case msg := <-server.sendAllCh:
			server.sendAll(msg)

		case err := <-server.errCh:
			logrus.Errorf("error (%v)", err)

		case <-server.doneCh:
			return
		}
	}
}

func (server *Server) sendNetwork(c *Client) {
	msg := &Message{}
	for id := range server.routers {
		msg.Routers = append(msg.Routers, id)
	}
	for _, link := range server.links {
		msg.Links = append(msg.Links, link)
	}
	c.Write(msg)
}

type Server struct {
	clients   map[int]*Client
	addCh     chan *Client
	delCh     chan *Client
	sendAllCh chan *Message
	doneCh    chan struct{}
	errCh     chan error
	routers   map[string]bool
	links     map[string]*Link
}
