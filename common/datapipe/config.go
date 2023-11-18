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
	"fmt"
	"github.com/gliderlabs/ssh"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/identity"
	gossh "golang.org/x/crypto/ssh"
	"os"
	"path"
	"strconv"
	"strings"
)

type LocalAccessType string

const (
	LocalAccessTypeNone              LocalAccessType = ""
	LocalAccessTypePort              LocalAccessType = "local-port"
	LocalAccessTypeEmbeddedSshServer LocalAccessType = "embedded-ssh-server"
)

type Config struct {
	Enabled            bool
	LocalAccessType    LocalAccessType // values: 'none', 'localhost:port', 'embedded'
	DestinationPort    uint16
	AuthorizedKeysFile string
	HostKey            gossh.Signer
	ShellPath          string
}

func (self *Config) IsLocalAccessAllowed() bool {
	return self.Enabled && self.LocalAccessType != LocalAccessTypeNone
}

func (self *Config) IsLocalPort() bool {
	return self.LocalAccessType == LocalAccessTypePort
}

func (self *Config) IsEmbedded() bool {
	return self.LocalAccessType == LocalAccessTypeEmbeddedSshServer
}

func (self *Config) LoadConfig(m map[interface{}]interface{}) error {
	log := pfxlog.Logger()
	if v, ok := m["enabled"]; ok {
		if enabled, ok := v.(bool); ok {
			self.Enabled = enabled
		} else {
			self.Enabled = strings.EqualFold("true", fmt.Sprintf("%v", v))
		}
	}
	if v, ok := m["enableExperimentalFeature"]; ok {
		if enabled, ok := v.(bool); ok {
			if !enabled {
				self.Enabled = false
			}
		} else if !strings.EqualFold("true", fmt.Sprintf("%v", v)) {
			self.Enabled = false
		}
	} else {
		self.Enabled = false
	}

	if self.Enabled {
		log.Infof("mgmt.pipe enabled")
		if v, ok := m["destination"]; ok {
			if destination, ok := v.(string); ok {
				if strings.HasPrefix(destination, "127.0.0.1:") {
					self.LocalAccessType = LocalAccessTypePort
					portStr := strings.TrimPrefix(destination, "127.0.0.1:")
					port, err := strconv.ParseUint(portStr, 10, 16)
					if err != nil {
						log.WithError(err).Warn("mgmt.pipe is enabled, but destination not valid. Must be '127.0.0.1:<port>' or 'embedded'")
						self.Enabled = false
						return nil
					}
					self.DestinationPort = uint16(port)
				} else if destination == "embedded-ssh-server" {
					self.LocalAccessType = LocalAccessTypeEmbeddedSshServer

					if v, ok = m["authorizedKeysFile"]; ok {
						if keysFile, ok := v.(string); ok {
							self.AuthorizedKeysFile = keysFile
						} else {
							log.Warnf("mgmt.pipe is enabled, but 'embedded' destination configured and authorizedKeysFile configuration is not type string, but %T", v)
							self.Enabled = false
							return nil
						}
					}

					if v, ok = m["shell"]; ok {
						if s, ok := v.(string); ok {
							self.ShellPath = s
						} else {
							log.Warnf("mgmt.pipe is enabled, but 'embedded' destination configured and shell configuration is not type string, but %T", v)
						}
					}
				} else {
					log.Warn("mgmt.pipe is enabled, but destination not valid. Must be 'localhost:port' or 'embedded'")
					self.Enabled = false
					return nil
				}
			}
		} else {
			self.Enabled = false
			log.Warn("mgmt.pipe is enabled, but destination not specified. mgmt.pipe disabled.")
			return nil
		}
	} else {
		log.Infof("mgmt.pipe disabled")
	}
	return nil
}

func (self *Config) NewSshRequestHandler(identity *identity.TokenId) (*SshRequestHandler, error) {
	if self.HostKey == nil {
		signer, err := gossh.NewSignerFromKey(identity.Cert().PrivateKey)
		if err != nil {
			return nil, err
		}
		self.HostKey = signer
	}

	keysFile := self.AuthorizedKeysFile
	if keysFile == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("could not set up ssh request handler, failing get home dir, trying to load default authorized keys (%w)", err)
		}
		keysFile = path.Join(homeDir, ".ssh", "authorized_keys")
	}

	keysFileContents, err := os.ReadFile(keysFile)
	if err != nil {
		return nil, fmt.Errorf("could not set up ssh request handler, failed to load authorized keys from '%s' (%w)", keysFile, err)
	}

	authorizedKeys := map[string]struct{}{}
	entryIdx := 0
	for len(keysFileContents) > 0 {
		pubKey, _, _, rest, err := gossh.ParseAuthorizedKey(keysFileContents)
		if err != nil {
			return nil, fmt.Errorf("could not set up ssh request handler, failed to load authorized key at index %d from '%s' (%w)", entryIdx, keysFile, err)
		}

		authorizedKeys[string(pubKey.Marshal())] = struct{}{}
		keysFileContents = rest
		entryIdx++
	}

	publicKeyOption := ssh.PublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
		_, found := authorizedKeys[string(key.Marshal())]
		return found
	})

	return &SshRequestHandler{
		config:  self,
		options: []ssh.Option{publicKeyOption},
	}, nil
}
