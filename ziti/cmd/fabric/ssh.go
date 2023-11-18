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

package fabric

import (
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
	"io"
	"net"
	"os"
	"os/user"
	"strings"
	"time"
)

type sshAction struct {
	api.Options

	incomingData chan []byte
	user         string
	keyPath      string
	proxyMode    bool
	done         chan struct{}

	connections concurrenz.CopyOnWriteSlice[*sshConn]
}

func NewSshCmd(p common.OptionsProvider) *cobra.Command {
	action := sshAction{
		Options: api.Options{
			CommonOptions: p(),
		},
		incomingData: make(chan []byte, 4),
		done:         make(chan struct{}),
	}

	sshCmd := &cobra.Command{
		Use:     "ssh <destination>",
		Short:   "ssh to ziti components",
		Example: "ziti fabric ssh ctrl",
		Args:    cobra.ExactArgs(1),
		RunE:    action.ssh,
	}

	action.AddCommonFlags(sshCmd)
	sshCmd.Flags().StringVarP(&action.user, "user", "u", "", "SSH username")
	sshCmd.Flags().StringVarP(&action.keyPath, "key", "k", "", "SSH key path")
	sshCmd.Flags().BoolVar(&action.proxyMode, "proxy-mode", false, "run in proxy mode, to be called from ssh")
	return sshCmd
}

func (self *sshAction) closeConnections() {
	for _, conn := range self.connections.Value() {
		pfxlog.Logger().Infof("closing ssh connection %d", conn.id)
		_ = conn.Close()
	}
	close(self.done)
}

func (self *sshAction) ssh(cmd *cobra.Command, args []string) error {
	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_MgmtPipeDataType), self.receiveData)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			self.closeConnections()
		}))
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_MgmtPipeCloseType), func(m *channel.Message, ch channel.Channel) {
			self.closeConnections()
		})
		return nil
	}

	ch, err := api.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		return err
	}

	destination := args[0]

	if idx := strings.IndexByte(destination, '@'); idx > 0 {
		self.user = destination[0:idx]
		destination = destination[idx+1:]
	}

	sshRequest := &mgmt_pb.MgmtPipeRequest{
		DestinationType: mgmt_pb.DestinationType_Any,
		Destination:     destination,
		TimeoutMillis:   uint64(self.Timeout * 1000),
	}

	if strings.HasPrefix(destination, "ctrl:") {
		sshRequest.DestinationType = mgmt_pb.DestinationType_Controller
		sshRequest.Destination = strings.TrimPrefix(destination, "ctrl:")
	} else if strings.HasPrefix(destination, "c:") {
		sshRequest.DestinationType = mgmt_pb.DestinationType_Controller
		sshRequest.Destination = strings.TrimPrefix(destination, "c:")
	} else if strings.HasPrefix(destination, "router:") {
		sshRequest.DestinationType = mgmt_pb.DestinationType_Router
		sshRequest.Destination = strings.TrimPrefix(destination, "router:")
	} else if strings.HasPrefix(destination, "r:") {
		sshRequest.DestinationType = mgmt_pb.DestinationType_Router
		sshRequest.Destination = strings.TrimPrefix(destination, "r:")
	}

	// router name -> router id mapping
	if sshRequest.DestinationType == mgmt_pb.DestinationType_Any || sshRequest.DestinationType == mgmt_pb.DestinationType_Router {
		id, err := api.MapNameToID(util.FabricAPI, "routers", &self.Options, sshRequest.Destination)
		if err == nil {
			sshRequest.DestinationType = mgmt_pb.DestinationType_Router
			sshRequest.Destination = id
		}
	}

	resp, err := protobufs.MarshalTyped(sshRequest).WithTimeout(time.Duration(self.Timeout) * time.Second).SendForReply(ch)
	sshResp := &mgmt_pb.MgmtPipeResponse{}
	err = protobufs.TypedResponse(sshResp).Unmarshall(resp, err)
	if err != nil {
		return err
	}

	if !sshResp.Success {
		return errors.New(sshResp.Msg)
	}

	conn := &sshConn{
		id:          sshResp.ConnId,
		ch:          ch,
		ReadAdapter: channel.NewReadAdapter(fmt.Sprintf("mgmt-pipe-%d", sshResp.ConnId), 4),
	}

	go func() {
		done := false
		for !done {
			select {
			case data := <-self.incomingData:
				if err := conn.PushData(data); err != nil {
					return
				}
			case <-self.done:
				done = true
			}
		}

		for {
			select {
			case data := <-self.incomingData:
				if err := conn.PushData(data); err != nil {
					return
				}
			default:
				break
			}
		}
	}()

	self.connections.Append(conn)

	if self.proxyMode {
		return self.runProxy(conn)
	}

	return self.remoteShell(conn)
}

func (self *sshAction) runProxy(conn net.Conn) error {
	errC := make(chan error, 2)
	go func() {
		_, err := io.Copy(conn, os.Stdin)
		errC <- err
	}()

	go func() {
		_, err := io.Copy(os.Stdout, conn)
		errC <- err
	}()

	err := <-errC
	if err == nil {
		select {
		case err = <-errC:
		default:
		}
	}

	return err
}

func sshAuthMethodFromFile(keyPath string) (ssh.AuthMethod, error) {
	content, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("could not read ssh file [%s]: %w", keyPath, err)
	}

	if signer, err := ssh.ParsePrivateKey(content); err == nil {
		return ssh.PublicKeys(signer), nil
	} else {
		if err.Error() == "ssh: no key found" {
			return nil, fmt.Errorf("no private key found in [%s]: %w", keyPath, err)
		} else if err.(*ssh.PassphraseMissingError) != nil {
			return nil, fmt.Errorf("file is password protected [%s] %w", keyPath, err)
		} else {
			return nil, fmt.Errorf("error parsing private key from [%s]L %w", keyPath, err)
		}
	}
}

func sshAuthMethodAgent() ssh.AuthMethod {
	if sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		return ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers)
	}
	return nil
}

func (self *sshAction) newSshConfig() *ssh.ClientConfig {
	var methods []ssh.AuthMethod

	if fileMethod, err := sshAuthMethodFromFile(self.keyPath); err == nil {
		methods = append(methods, fileMethod)
	} else {
		logrus.Error(err)
	}

	if agentMethod := sshAuthMethodAgent(); agentMethod != nil {
		methods = append(methods, sshAuthMethodAgent())
	}

	return &ssh.ClientConfig{
		User:            self.user,
		Auth:            methods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
}

func (self *sshAction) remoteShell(conn net.Conn) error {
	if self.user == "" {
		current, err := user.Current()
		if err != nil {
			return fmt.Errorf("unable to get current user: %w", err)
		}
		self.user = current.Name
	}

	clientConfig := self.newSshConfig()
	c, chans, reqs, err := ssh.NewClientConn(conn, "localhost:22", clientConfig)
	if err != nil {
		return err
	}
	client := ssh.NewClient(c, chans, reqs)

	session, err := client.NewSession()
	if err != nil {
		return err
	}

	fd := int(os.Stdout.Fd())

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = session.Close()
		_ = term.Restore(fd, oldState)
	}()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	session.Stdin = os.Stdin

	termWidth, termHeight, err := term.GetSize(fd)
	if err != nil {
		panic(err)
	}

	if err := session.RequestPty("xterm", termHeight, termWidth, ssh.TerminalModes{ssh.ECHO: 1}); err != nil {
		return err
	}

	return session.Run("/bin/bash")
}

func (self *sshAction) receiveData(msg *channel.Message, _ channel.Channel) {
	self.incomingData <- msg.Body
}

type sshConn struct {
	id uint32
	ch channel.Channel
	*channel.ReadAdapter
}

func (self *sshConn) Write(b []byte) (n int, err error) {
	msg := channel.NewMessage(int32(mgmt_pb.ContentType_MgmtPipeDataType), b)
	msg.PutUint32Header(int32(mgmt_pb.Header_MgmtPipeIdHeader), self.id)
	if err = msg.WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch); err != nil {
		return 0, err
	}
	return len(b), err
}

func (self *sshConn) Close() error {
	self.ReadAdapter.Close()
	return self.ch.Close()
}

func (self *sshConn) LocalAddr() net.Addr {
	return self.ch.Underlay().GetLocalAddr()
}

func (self *sshConn) RemoteAddr() net.Addr {
	return sshAddr{
		destination: fmt.Sprintf("%v", self.id),
	}
}

func (self *sshConn) SetDeadline(t time.Time) error {
	//TODO implement me
	panic("implement me")
}

func (self *sshConn) SetWriteDeadline(t time.Time) error {
	//TODO implement me
	panic("implement me")
}

type sshAddr struct {
	destination string
}

func (self sshAddr) Network() string {
	return "ziti"
}

func (self sshAddr) String() string {
	return "ziti:" + self.destination
}
