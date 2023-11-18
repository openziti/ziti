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

package datapipe

import (
	"context"
	"errors"
	"fmt"
	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
	"github.com/michaelquigley/pfxlog"
	"github.com/pkg/sftp"
	"io"
	"net"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

type SshRequestHandler struct {
	config  *Config
	options []ssh.Option
}

func (self *SshRequestHandler) HandleSshRequest(conn *EmbeddedSshConn) error {
	log := pfxlog.Logger()

	sftpHandler := func(s ssh.Session) {
		sftpServer, err := sftp.NewServer(s)
		if err != nil {
			log.WithError(err).Error("error initializing sftp server")
			return
		}

		if err = sftpServer.Serve(); err == io.EOF {
			if closeErr := sftpServer.Close(); closeErr != nil {
				log.WithError(closeErr).Error("error closing sftp server")
			}
			log.Info("sftp client exited session")
		} else if err != nil {
			log.WithError(err).Error("sftp server completed with error")
		}
	}

	server := &ssh.Server{
		Handler: func(s ssh.Session) {
			conn.SetSshConn(s)
			log.Infof("requested subsystem: %s, cmd: %s", s.Subsystem(), s.RawCommand())
			if _, _, hasPty := s.Pty(); hasPty {
				log.Infof("pty requested")
				self.startPty(s)
			} else {
				self.startNonPty(s)
			}
		},
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": sftpHandler,
		},
	}

	server.AddHostKey(self.config.HostKey)
	for _, option := range self.options {
		if err := server.SetOption(option); err != nil {
			return err
		}
	}

	l := &singleServingListener{
		conn: conn,
	}

	go func() {
		err := server.Serve(l)
		if err != nil && !l.IsComplete() {
			pfxlog.Logger().WithError(err).Error("ssh server finished")
		}
	}()

	return nil
}

func (self *SshRequestHandler) startPty(s ssh.Session) {
	log := pfxlog.Logger()

	cmd, cancelF := self.getCommand(s)
	defer cancelF()

	ptyx, winC, _ := s.Pty()
	cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyx.Term))
	f, err := pty.StartWithSize(cmd, &pty.Winsize{
		X: uint16(ptyx.Window.Width),
		Y: uint16(ptyx.Window.Height),
	})

	if err != nil {
		log.WithError(err).Error("error getting pty for ssh shell")
		return
	}

	log.Info("pty allocated for ssh shell")

	done := make(chan struct{})

	defer func() {
		close(done)
		if err := f.Close(); err != nil {
			log.WithError(err).Error("error closing pty for ssh shell")
		}
		log.Info("exiting pty shell")
	}()

	go self.handleWindowSizes(winC, f, done)

	errC := make(chan error, 2)
	go func() {
		_, err := io.Copy(s, f)
		errC <- err
	}()

	go func() {
		_, err := io.Copy(f, s)
		errC <- err
	}()

	err = <-errC
	if err == nil {
		select {
		case err = <-errC:
		default:
		}
	}

	if err != nil {
		log.WithError(err).Error("error reported from ssh shell io copy")
		return
	}

	if err = cmd.Wait(); err != nil {
		log.WithError(err).Error("error reported from ssh shell wait-for-exit")
		return
	}
}

func (self *SshRequestHandler) handleWindowSizes(winC <-chan ssh.Window, f *os.File, done <-chan struct{}) {
	log := pfxlog.Logger()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var size *ssh.Window

	for {
		select {
		case nextSize := <-winC:
			size = &nextSize
		case <-ticker.C:
			if size != nil {
				newSize := &pty.Winsize{
					Rows: uint16(size.Height),
					Cols: uint16(size.Width),
				}
				if err := pty.Setsize(f, newSize); err != nil {
					log.WithError(err).Error("error setting pty size for ssh shell")
				}
			}
			size = nil
		case <-done:
			return
		}
	}
}

func (self *SshRequestHandler) startNonPty(s ssh.Session) {
	log := pfxlog.Logger()

	cmd, cancelF := self.getCommand(s)
	defer cancelF()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.WithError(err).Error("error getting stdout pipe for ssh shell")
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.WithError(err).Error("error getting stderr pipe for ssh shell")
		return
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.WithError(err).Error("error getting stdin pipe for ssh shell")
		return
	}

	if err = cmd.Start(); err != nil {
		return
	}

	errC := make(chan error, 3)
	go func() {
		_, err := io.Copy(s, stdout)
		errC <- err
	}()

	go func() {
		_, err := io.Copy(stdin, s)
		errC <- err
	}()

	go func() {
		_, err := io.Copy(s.Stderr(), stderr)
		errC <- err
	}()

	for i := 0; i < 3; i++ {
		err = <-errC
		if err != nil {
			break
		}
	}

	if err != nil {
		log.WithError(err).Error("error reported from ssh shell io copy")
		return
	}

	if err = cmd.Wait(); err != nil {
		log.WithError(err).Error("error reported from ssh shell wait-for-exit")
		return
	}
}

func (self *SshRequestHandler) getCommand(s ssh.Session) (*exec.Cmd, func()) {
	var executable string
	var args []string

	if cmdLine := s.Command(); len(cmdLine) > 0 {
		executable = cmdLine[0]
		if len(cmdLine) > 1 {
			args = cmdLine[1:]
		}
	} else {
		executable = self.config.ShellPath
		if executable == "" {
			executable = "/bin/sh"
		}
	}

	ctx, cancelF := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, executable, args...)
	return cmd, cancelF
}

type singleServingListener struct {
	served   atomic.Bool
	conn     net.Conn
	complete atomic.Bool
}

func (self *singleServingListener) Network() string {
	return "ziti-ssa"
}

func (self *singleServingListener) String() string {
	return self.conn.LocalAddr().String()
}

func (self *singleServingListener) Accept() (net.Conn, error) {
	if self.served.CompareAndSwap(false, true) {
		self.complete.Store(true)
		return self.conn, nil
	}
	return nil, errors.New("closed")
}

func (self *singleServingListener) IsComplete() bool {
	return self.complete.Load()
}

func (self *singleServingListener) Close() error {
	self.served.Store(true)
	return nil
}

func (self *singleServingListener) Addr() net.Addr {
	return self
}

//func (self *SshRequestHandler) connLoop(chans <-chan ssh.NewChannel, reqs <-chan *ssh.Request) {
//	log := pfxlog.Logger()
//
//	var channelRequests <-chan *ssh.Request
//	var channel ssh.Channel
//
//	for {
//		select {
//		case req := <-reqs:
//			if req.WantReply {
//				if err := req.Reply(false, nil); err != nil {
//					log.WithError(err).Error("error replying to ssh request")
//				}
//			}
//		case req, ok := <-channelRequests:
//			if !ok {
//				channelRequests = nil
//				if err := channel.Close(); err != nil {
//					log.WithError(err).Error("error closing embedded ssh channel")
//				}
//			} else {
//				log.WithField("reqType", req.Type).Debug("handling ssh request")
//				handled := false
//				if req.Type == "shell" {
//					go self.exec(channel)
//					handled = true
//				} else if req.Type == "exec" {
//					log.WithField("payload", req.Payload).Debug("handling exec")
//					command := string(req.Payload[4 : req.Payload[3]+4])
//					go self.exec(channel, "-c", command)
//					handled = true
//				} else if req.Type == "pty-req" {
//					handled = true
//				}
//
//				if req.WantReply {
//					if err := req.Reply(handled, nil); err != nil {
//						log.WithError(err).Error("error replying to channel ssh request")
//					}
//				}
//			}
//		case newChannel, ok := <-chans:
//			if !ok {
//				return
//			}
//
//			if newChannel.ChannelType() != "session" {
//				if err := newChannel.Reject(ssh.UnknownChannelType, "unknown channel type"); err != nil {
//					log.WithError(err).WithField("channelType", newChannel.ChannelType()).
//						Error("error sending ssh channel reject for channel type")
//				}
//			} else if channelRequests != nil {
//				if err := newChannel.Reject(ssh.ResourceShortage, "only one connection allowed at a time"); err != nil {
//					log.WithError(err).Error("error sending ssh channel reject for additional channel")
//				}
//			} else {
//				var err error
//				channel, channelRequests, err = newChannel.Accept()
//				if err != nil {
//					log.WithError(err).WithField("type", newChannel.ChannelType()).Error("error accepting ssh channel")
//					continue
//				}
//			}
//		}
//	}
//}
//
//func (self *SshRequestHandler) exec(ch ssh.Channel, args ...string) {
//	log := pfxlog.Logger()
//	defer func() {
//		if _, err := ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0}); err != nil {
//			log.WithError(err).Error("error sending ssh exit-status request")
//		}
//	}()
//
//	shellPath := self.config.ShellPath
//	if shellPath == "" {
//		shellPath = "/bin/sh"
//	}
//	cmd := exec.Command(shellPath, args...)
//	f, err := pty.Start(cmd)
//	if err != nil {
//		log.WithError(err).Error("error getting stdout pipe for ssh shell")
//		return
//	}
//
//	defer func() {
//		f.Close()
//	}()
//
//	//stdout, err := cmd.StdoutPipe()
//	//if err != nil {
//	//	log.WithError(err).Error("error getting stdout pipe for ssh shell")
//	//	return
//	//}
//	//stderr, err := cmd.StderrPipe()
//	//if err != nil {
//	//	log.WithError(err).Error("error getting stderr pipe for ssh shell")
//	//	return
//	//}
//	//input, err := cmd.StdinPipe()
//	//if err != nil {
//	//	log.WithError(err).Error("error getting stdin pipe for ssh shell")
//	//	return
//	//}
//	//
//	//if err = cmd.Start(); err != nil {
//	//	return
//	//}
//
//	go io.Copy(f, ch)
//	//go io.Copy(ch.Stderr(), stderr)
//	io.Copy(ch, f)
//
//	if err = cmd.Wait(); err != nil {
//		log.WithError(err).Error("error reported from ssh shell wait-for-exit")
//		return
//	}
//}
