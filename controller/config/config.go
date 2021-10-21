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

package config

import (
	"bytes"
	"crypto/sha1"
	"encoding/pem"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/identity/identity"
	"github.com/pkg/errors"
	"io/ioutil"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	sessionTimeoutDefault = 10 * time.Minute
	sessionTimeoutMin     = 1 * time.Minute

	enrollmentDurationMin     = 5 * time.Minute
	enrollmentDurationDefault = 5 * time.Minute
)

type Enrollment struct {
	SigningCert       identity.Identity
	SigningCertConfig identity.Config
	SigningCertCaPem  []byte
	EdgeIdentity      EnrollmentOption
	EdgeRouter        EnrollmentOption
}

type EnrollmentOption struct {
	Duration time.Duration
}

type Api struct {
	SessionTimeout          time.Duration
	ActivityUpdateBatchSize int
	ActivityUpdateInterval  time.Duration

	Listener      string
	Address       string
	IdentityCaPem []byte
	HttpTimeouts  HttpTimeouts
}

type Config struct {
	Enabled            bool
	Api                Api
	Enrollment         Enrollment

	caPems     *bytes.Buffer
	caPemsOnce sync.Once
}

type HttpTimeouts struct {
	ReadTimeoutDuration       time.Duration
	ReadHeaderTimeoutDuration time.Duration
	WriteTimeoutDuration      time.Duration
	IdleTimeoutsDuration      time.Duration
}

func NewConfig() *Config {
	return &Config{
		Enabled: false,
		caPems:  bytes.NewBuffer(nil),
	}
}

func (c *Config) SessionTimeoutDuration() time.Duration {
	return c.Api.SessionTimeout
}

func (c *Config) CaPems() []byte {
	c.caPemsOnce.Do(func() {
		c.RefreshCaPems()
	})

	return c.caPems.Bytes()
}

// AddCaPems adds a byte array of certificates to the current buffered list of CAs. The certificates
// should be in PEM format separated by new lines. RefreshCaPems should be called after all
// calls to AddCaPems are completed.
func (c *Config) AddCaPems(caPems []byte) {
	c.caPems.WriteString("\n")
	c.caPems.Write(caPems)
}

func (c *Config) RefreshCaPems() {
	c.caPems = CalculateCaPems(c.caPems)
}


func (c *Config) loadApiSection(edgeConfigMap map[interface{}]interface{}) error {
	c.Api = Api{}
	var err error

	c.Api.ActivityUpdateBatchSize = 250
	c.Api.ActivityUpdateInterval = 90 * time.Second

	if value, found := edgeConfigMap["api"]; found {
		apiSubMap := value.(map[interface{}]interface{})

		if val, ok := apiSubMap["address"]; ok {
			if c.Api.Address, ok = val.(string); !ok {
				return errors.Errorf("invalid type %t for [edge.api.address], must be string", val)
			}

			if c.Api.Address == "" {
				return errors.Errorf("invalid type %t for [edge.api.address], must not be an empty string", val)
			}

			if err := validateHostPortString(c.Api.Address); err != nil {
				return errors.Errorf("invalid value %s for [edge.api.address]: %v", c.Api.Address, err)
			}
		} else {
			return errors.New("required value [edge.api.address] is required")
		}

		var durationValue = 0 * time.Second
		if value, found := apiSubMap["sessionTimeout"]; found {
			strValue := value.(string)
			durationValue, err = time.ParseDuration(strValue)
			if err != nil {
				return errors.Errorf("error parsing [edge.api.sessionTimeout], invalid duration string %s, cannot parse as duration (e.g. 1m): %v", strValue, err)
			}
		}

		if durationValue < sessionTimeoutMin {
			durationValue = sessionTimeoutDefault
			pfxlog.Logger().Warnf("[edge.api.sessionTimeout] defaulted to %v", durationValue)
		}

		c.Api.SessionTimeout = durationValue

		if val, ok := apiSubMap["activityUpdateBatchSize"]; ok {
			if c.Api.ActivityUpdateBatchSize, ok = val.(int); !ok {
				return errors.Errorf("invalid type %v for apiSessions.activityUpdateBatchSize, must be int", reflect.TypeOf(val))
			}
		}

		if val, ok := apiSubMap["activityUpdateInterval"]; ok {
			if strVal, ok := val.(string); !ok {
				return errors.Errorf("invalid type %v for apiSessions.activityUpdateInterval, must be string duration", reflect.TypeOf(val))
			} else {
				if c.Api.ActivityUpdateInterval, err = time.ParseDuration(strVal); err != nil {
					return errors.Wrapf(err, "invalid value %v for apiSessions.activityUpdateInterval, must be string duration", val)
				}
			}
		}

		if c.Api.ActivityUpdateBatchSize < 1 || c.Api.ActivityUpdateBatchSize > 10000 {
			return errors.Errorf("invalid value %v for apiSessions.activityUpdateBatchSize, must be between 1 and 10000", c.Api.ActivityUpdateBatchSize)
		}

		if c.Api.ActivityUpdateInterval < time.Millisecond || c.Api.ActivityUpdateInterval > 10*time.Minute {
			return errors.Errorf("invalid value %v for apiSessions.activityUpdateInterval, must be between 1ms and 10m", c.Api.ActivityUpdateInterval.String())
		}

		return nil

	} else {
		return errors.New("required configuration section [edge.api] missing")
	}
}

func validateHostPortString(address string) error {
	address = strings.TrimSpace(address)

	if address == "" {
		return errors.New("must not be an empty string or unspecified")
	}

	host, port, err := net.SplitHostPort(address)

	if err != nil {
		return errors.Errorf("could not split host and port: %v", err)
	}

	if host == "" {
		return errors.New("host must be specified")
	}

	if port == "" {
		return errors.New("port must be specified")
	}

	if port, err := strconv.ParseInt(port, 10, 32); err != nil {
		return errors.New("invalid port, must be a integer")
	} else if port < 1 || port > 65535 {
		return errors.New("invalid port, must 1-65535")
	}

	return nil
}

func (c *Config) loadEnrollmentSection(edgeConfigMap map[interface{}]interface{}) error {
	c.Enrollment = Enrollment{}
	var err error

	if value, found := edgeConfigMap["enrollment"]; found {
		enrollmentSubMap := value.(map[interface{}]interface{})

		if value, found := enrollmentSubMap["signingCert"]; found {
			signingCertSubMap := value.(map[interface{}]interface{})
			c.Enrollment.SigningCertConfig = identity.Config{}

			if value, found := signingCertSubMap["cert"]; found {
				c.Enrollment.SigningCertConfig.Cert = value.(string)
				certPem, err := ioutil.ReadFile(c.Enrollment.SigningCertConfig.Cert)
				if err != nil {
					pfxlog.Logger().WithError(err).Panic("unable to read [edge.enrollment.cert]")
				}
				//The signer is a valid trust anchor
				_, _ = c.caPems.WriteString("\n")
				_, _ = c.caPems.Write(certPem)

			} else {
				return fmt.Errorf("required configuration value [edge.enrollment.cert] is missing")
			}

			if value, found := signingCertSubMap["key"]; found {
				c.Enrollment.SigningCertConfig.Key = value.(string)
			} else {
				return fmt.Errorf("required configuration value [edge.enrollment.key] is missing")
			}

			if value, found := signingCertSubMap["ca"]; found {
				c.Enrollment.SigningCertConfig.CA = value.(string)

				if c.Enrollment.SigningCertCaPem, err = ioutil.ReadFile(c.Enrollment.SigningCertConfig.CA); err != nil {
					return fmt.Errorf("could not read file CA file from [edge.enrollment.signingCert.ca]")
				}

				_, _ = c.caPems.WriteString("\n")
				_, _ = c.caPems.Write(c.Enrollment.SigningCertCaPem)
			} //not an error if the signing certificate's CA is already represented in the root [identity.ca]

			if c.Enrollment.SigningCert, err = identity.LoadIdentity(c.Enrollment.SigningCertConfig); err != nil {
				return fmt.Errorf("error loading [edge.enrollment.signingCert]: %s", err)
			}

		} else {
			return errors.New("required configuration section [edge.enrollment.signingCert] missing")
		}

		if value, found := enrollmentSubMap["edgeIdentity"]; found {
			edgeIdentitySubMap := value.(map[interface{}]interface{})

			edgeIdentityDuration := 0 * time.Second
			if value, found := edgeIdentitySubMap["duration"]; found {
				strValue := value.(string)
				var err error
				edgeIdentityDuration, err = time.ParseDuration(strValue)

				if err != nil {
					return errors.Errorf("error parsing [edge.enrollment.edgeIdentity.duration], invalid duration string %s, cannot parse as duration (e.g. 1m): %v", strValue, err)
				}
			}

			if edgeIdentityDuration < enrollmentDurationMin {
				edgeIdentityDuration = enrollmentDurationDefault
			}

			c.Enrollment.EdgeIdentity = EnrollmentOption{Duration: edgeIdentityDuration}

		} else {
			return errors.New("required configuration section [edge.enrollment.edgeIdentity] missing")
		}

		if value, found := enrollmentSubMap["edgeRouter"]; found {
			edgeRouterSubMap := value.(map[interface{}]interface{})

			edgeRouterDuration := 0 * time.Second
			if value, found := edgeRouterSubMap["duration"]; found {
				strValue := value.(string)
				var err error
				edgeRouterDuration, err = time.ParseDuration(strValue)

				if err != nil {
					return errors.Errorf("error parsing [edge.enrollment.edgeRouter.duration], invalid duration string %s, cannot parse as duration (e.g. 1m): %v", strValue, err)
				}
			}

			if edgeRouterDuration < enrollmentDurationMin {
				edgeRouterDuration = enrollmentDurationDefault
			}

			c.Enrollment.EdgeRouter = EnrollmentOption{Duration: edgeRouterDuration}

		} else {
			return errors.New("required configuration section [edge.enrollment.edgeRouter] missing")
		}

	} else {
		return errors.New("required configuration section [edge.enrollment] missing")
	}

	return nil
}

func LoadFromMap(configMap map[interface{}]interface{}) (*Config, error) {
	edgeConfig := NewConfig()

	var edgeConfigMap map[interface{}]interface{}

	if val, ok := configMap["edge"]; ok && val != nil {
		if edgeConfigMap, ok = val.(map[interface{}]interface{}); !ok {
			return nil, fmt.Errorf("expected map as edge configuration")
		}
	} else {
		return edgeConfig, nil
	}

	edgeConfig.Enabled = configMap != nil

	if !edgeConfig.Enabled {
		return edgeConfig, nil
	}

	var err error

	if err = edgeConfig.loadApiSection(edgeConfigMap); err != nil {
		return nil, err
	}

	if err = edgeConfig.loadEnrollmentSection(edgeConfigMap); err != nil {
		return nil, err
	}

	return edgeConfig, nil
}

// CalculateCaPems takes the supplied caPems buffer as a set of PEM Certificates separated by new lines. Duplicate
// certificates are removed and the result is returned as a bytes.Buffer of PEM Certificates separated by new lines.
func CalculateCaPems(caPems *bytes.Buffer) *bytes.Buffer {
	caPemMap := map[string][]byte{}

	newCaPems := bytes.Buffer{}
	blocksToProcess := caPems.Bytes()
	for len(blocksToProcess) != 0 {
		var block *pem.Block
		block, blocksToProcess = pem.Decode(blocksToProcess)

		if block != nil {
			// #nosec
			hash := sha1.Sum(block.Bytes)
			fingerprint := toHex(hash[:])
			newPem := pem.EncodeToMemory(block)
			caPemMap[fingerprint] = newPem
			_, _ = newCaPems.WriteString("\n")
			_, _ = newCaPems.Write(newPem)
		}
	}

	return &newCaPems
}

// toHex takes a byte array returns a hex formatted fingerprint
func toHex(data []byte) string {
	var buf bytes.Buffer
	for i, b := range data {
		if i > 0 {
			_, _ = fmt.Fprintf(&buf, ":")
		}
		_, _ = fmt.Fprintf(&buf, "%02x", b)
	}
	return strings.ToUpper(buf.String())
}
