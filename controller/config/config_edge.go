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

package config

import (
	"bytes"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/controller/command"
	"github.com/pkg/errors"
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultEdgeApiActivityUpdateBatchSize = 250
	DefaultEdgeAPIActivityUpdateInterval  = 90 * time.Second
	MaxEdgeAPIActivityUpdateBatchSize     = 10000
	MinEdgeAPIActivityUpdateBatchSize     = 1
	MaxEdgeAPIActivityUpdateInterval      = 10 * time.Minute
	MinEdgeAPIActivityUpdateInterval      = time.Millisecond

	DefaultEdgeSessionTimeout = 30 * time.Minute
	MinEdgeSessionTimeout     = 1 * time.Minute

	MinEdgeEnrollmentDuration     = 5 * time.Minute
	DefaultEdgeEnrollmentDuration = 180 * time.Minute

	DefaultHttpIdleTimeout       = 5000 * time.Millisecond
	DefaultHttpReadTimeout       = 5000 * time.Millisecond
	DefaultHttpReadHeaderTimeout = 5000 * time.Millisecond
	DefaultHttpWriteTimeout      = 100000 * time.Millisecond

	DefaultTotpDomain = "openziti.io"

	DefaultAuthRateLimiterEnabled = true
	DefaultAuthRateLimiterMaxSize = 250
	DefaultAuthRateLimiterMinSize = 5

	AuthRateLimiterMinSizeValue = 5
	AuthRateLimiterMaxSizeValue = 1000

	DefaultIdentityOnlineStatusScanInterval = time.Minute
	MinIdentityOnlineStatusScanInterval     = time.Second

	DefaultIdentityOnlineStatusUnknownTimeout = 5 * time.Minute
	DefaultIdentityOnlineStatusSource         = IdentityStatusSourceHybrid
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

type Totp struct {
	Hostname string
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

type EdgeConfig struct {
	Enabled              bool
	Api                  Api
	Enrollment           Enrollment
	IdentityStatusConfig IdentityStatusConfig
	caPems               *bytes.Buffer
	caPemsOnce           sync.Once
	Totp                 Totp
	AuthRateLimiter      command.AdaptiveRateLimiterConfig
	caCerts              []*x509.Certificate
	caCertPool           *x509.CertPool
}

type HttpTimeouts struct {
	ReadTimeoutDuration       time.Duration
	ReadHeaderTimeoutDuration time.Duration
	WriteTimeoutDuration      time.Duration
	IdleTimeoutsDuration      time.Duration
}

func DefaultHttpTimeouts() *HttpTimeouts {
	httpTimeouts := &HttpTimeouts{
		ReadTimeoutDuration:       DefaultHttpReadTimeout,
		ReadHeaderTimeoutDuration: DefaultHttpReadHeaderTimeout,
		WriteTimeoutDuration:      DefaultHttpWriteTimeout,
		IdleTimeoutsDuration:      DefaultHttpIdleTimeout,
	}
	return httpTimeouts
}

type IdentityStatusSource uint32

const (
	IdentityStatusSourceHeartbeats    IdentityStatusSource = 1
	IdentityStatusSourceConnectEvents IdentityStatusSource = 2
	IdentityStatusSourceHybrid        IdentityStatusSource = 3
)

type IdentityStatusConfig struct {
	Source         IdentityStatusSource
	ScanInterval   time.Duration
	UnknownTimeout time.Duration
}

func NewEdgeConfig() *EdgeConfig {
	return &EdgeConfig{
		Enabled: false,
		caPems:  bytes.NewBuffer(nil),
	}
}

func (c *EdgeConfig) SessionTimeoutDuration() time.Duration {
	return c.Api.SessionTimeout
}

func (c *EdgeConfig) CaPems() []byte {
	c.caPemsOnce.Do(func() {
		c.RefreshCas()
	})

	return c.caPems.Bytes()
}

func (c *EdgeConfig) CaCerts() []*x509.Certificate {
	c.caPemsOnce.Do(func() {
		c.RefreshCas()
	})

	return c.caCerts
}

func (c *EdgeConfig) CaCertsPool() *x509.CertPool {
	c.caPemsOnce.Do(func() {
		c.RefreshCas()
	})

	return c.caCertPool
}

// AddCaPems adds a byte array of certificates to the current buffered list of CAs. The certificates
// should be in PEM format separated by new lines. RefreshCas should be called after all
// calls to AddCaPems are completed.
func (c *EdgeConfig) AddCaPems(caPems []byte) {
	c.caPems.WriteString("\n")
	c.caPems.Write(caPems)
}

func (c *EdgeConfig) RefreshCas() {
	c.caPems = CalculateCaPems(c.caPems)
	c.caCerts = nfpem.PemBytesToCertificates(c.caPems.Bytes())
	c.caCertPool = x509.NewCertPool()

	for _, cert := range c.caCerts {
		c.caCertPool.AddCert(cert)
	}
}

func (c *EdgeConfig) loadTotpSection(edgeConfigMap map[any]any) error {
	c.Totp = Totp{}
	c.Totp.Hostname = DefaultTotpDomain

	if value, found := edgeConfigMap["totp"]; found {
		if value == nil {
			return nil
		}

		totpMap := value.(map[interface{}]interface{})

		if totpMap != nil {
			if hostnameVal, found := totpMap["hostname"]; found {

				if hostnameVal == nil {
					return nil
				}

				if hostname, ok := hostnameVal.(string); ok {
					testUrl := "https://" + hostname
					parsedUrl, err := url.Parse(testUrl)

					if err != nil {
						return fmt.Errorf("could not parse URL: %w", err)
					}

					if parsedUrl.Hostname() != hostname {
						return fmt.Errorf("invalid hostname in [edge.totp.hostname]: %s", hostname)
					}

					c.Totp.Hostname = hostname
				} else {
					return fmt.Errorf("[edge.totp.hostname] must be a string")
				}
			}
		}
	}

	return nil
}

func (c *EdgeConfig) loadApiSection(edgeConfigMap map[interface{}]interface{}) error {
	c.Api = Api{}
	c.Api.HttpTimeouts = *DefaultHttpTimeouts()
	var err error

	c.Api.ActivityUpdateBatchSize = DefaultEdgeApiActivityUpdateBatchSize
	c.Api.ActivityUpdateInterval = DefaultEdgeAPIActivityUpdateInterval

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

		if durationValue < MinEdgeSessionTimeout {
			durationValue = DefaultEdgeSessionTimeout
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

		if c.Api.ActivityUpdateBatchSize < MinEdgeAPIActivityUpdateBatchSize || c.Api.ActivityUpdateBatchSize > MaxEdgeAPIActivityUpdateBatchSize {
			return errors.Errorf("invalid value %v for apiSessions.activityUpdateBatchSize, must be between %v and %v", c.Api.ActivityUpdateBatchSize, MinEdgeAPIActivityUpdateBatchSize, MaxEdgeAPIActivityUpdateBatchSize)
		}

		if c.Api.ActivityUpdateInterval < MinEdgeAPIActivityUpdateInterval || c.Api.ActivityUpdateInterval > MaxEdgeAPIActivityUpdateInterval {
			return errors.Errorf("invalid value %v for apiSessions.activityUpdateInterval, must be between %vms and %vm", c.Api.ActivityUpdateInterval.String(), MinEdgeAPIActivityUpdateInterval.Milliseconds(), MaxEdgeAPIActivityUpdateInterval.Minutes())
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

func (c *EdgeConfig) loadEnrollmentSection(edgeConfigMap map[interface{}]interface{}) error {
	c.Enrollment = Enrollment{}
	var err error

	if value, found := edgeConfigMap["enrollment"]; found {
		enrollmentSubMap := value.(map[interface{}]interface{})

		if value, found := enrollmentSubMap["signingCert"]; found {
			signingCertSubMap := value.(map[interface{}]interface{})
			c.Enrollment.SigningCertConfig = identity.Config{}

			if value, found := signingCertSubMap["cert"]; found {
				c.Enrollment.SigningCertConfig.Cert = value.(string)
				certPem, err := os.ReadFile(c.Enrollment.SigningCertConfig.Cert)
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

				if c.Enrollment.SigningCertCaPem, err = os.ReadFile(c.Enrollment.SigningCertConfig.CA); err != nil {
					return fmt.Errorf("could not read file CA file from [edge.enrollment.signingCert.ca]")
				}

				_, _ = c.caPems.WriteString("\n")
				_, _ = c.caPems.Write(c.Enrollment.SigningCertCaPem)
			} //not an error if the signing certificate's CA is already represented in the root [identity.ca]

			if c.Enrollment.SigningCert, err = identity.LoadIdentity(c.Enrollment.SigningCertConfig); err != nil {
				return fmt.Errorf("error loading [edge.enrollment.signingCert]: %s", err)
			} else {
				if err := c.Enrollment.SigningCert.WatchFiles(); err != nil {
					pfxlog.Logger().Warn("could not enable file watching on enrollment signing cert: %w", err)
				}
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

			if edgeIdentityDuration < MinEdgeEnrollmentDuration {
				edgeIdentityDuration = DefaultEdgeEnrollmentDuration
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

			if edgeRouterDuration < MinEdgeEnrollmentDuration {
				edgeRouterDuration = DefaultEdgeEnrollmentDuration
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

func (c *EdgeConfig) loadAuthRateLimiterConfig(cfgmap map[interface{}]interface{}) error {
	c.AuthRateLimiter.SetDefaults()

	c.AuthRateLimiter.Enabled = DefaultAuthRateLimiterEnabled
	c.AuthRateLimiter.MaxSize = DefaultAuthRateLimiterMaxSize
	c.AuthRateLimiter.MinSize = DefaultAuthRateLimiterMinSize

	if value, found := cfgmap["authRateLimiter"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if err := command.LoadAdaptiveRateLimiterConfig(&c.AuthRateLimiter, submap); err != nil {
				return err
			}
			if c.AuthRateLimiter.MaxSize < AuthRateLimiterMinSizeValue {
				return errors.Errorf("invalid value %v for authRateLimiter.maxSize, must be at least %v",
					c.AuthRateLimiter.MaxSize, AuthRateLimiterMinSizeValue)
			}
			if c.AuthRateLimiter.MaxSize > AuthRateLimiterMaxSizeValue {
				return errors.Errorf("invalid value %v for authRateLimiter.maxSize, must be at most %v",
					c.AuthRateLimiter.MaxSize, AuthRateLimiterMaxSizeValue)
			}

			if c.AuthRateLimiter.MinSize < AuthRateLimiterMinSizeValue {
				return errors.Errorf("invalid value %v for authRateLimiter.minSize, must be at least %v",
					c.AuthRateLimiter.MinSize, AuthRateLimiterMinSizeValue)
			}
			if c.AuthRateLimiter.MinSize > AuthRateLimiterMaxSizeValue {
				return errors.Errorf("invalid value %v for authRateLimiter.minSize, must be at most %v",
					c.AuthRateLimiter.MinSize, AuthRateLimiterMaxSizeValue)
			}
		} else {
			return errors.Errorf("invalid type for authRateLimiter, should be map instead of %T", value)
		}
	}

	return nil
}

func (c *EdgeConfig) loadIdentityStatusConfig(cfgmap map[interface{}]interface{}) error {
	c.IdentityStatusConfig.ScanInterval = DefaultIdentityOnlineStatusScanInterval
	c.IdentityStatusConfig.UnknownTimeout = DefaultIdentityOnlineStatusUnknownTimeout
	c.IdentityStatusConfig.Source = DefaultIdentityOnlineStatusSource

	if value, found := cfgmap["identityStatusConfig"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["scanInterval"]; found {
				if interval, err := time.ParseDuration(fmt.Sprintf("%v", value)); err != nil {
					pfxlog.Logger().WithError(err).Errorf("invalid value '%v' for identity status config scan interval", value)
				} else {
					c.IdentityStatusConfig.ScanInterval = interval
				}
			}

			if c.IdentityStatusConfig.ScanInterval < MinIdentityOnlineStatusScanInterval {
				pfxlog.Logger().Errorf("invalid value '%v' for identity status config scan interval, must be at least %s",
					c.IdentityStatusConfig.ScanInterval, MinIdentityOnlineStatusScanInterval)
				c.IdentityStatusConfig.ScanInterval = MinIdentityOnlineStatusScanInterval
			}

			if value, found := submap["unknownTimeout"]; found {
				if interval, err := time.ParseDuration(fmt.Sprintf("%v", value)); err != nil {
					pfxlog.Logger().WithError(err).Errorf("invalid value '%v' for identity status config unknown timeout", value)
				} else {
					c.IdentityStatusConfig.UnknownTimeout = interval
				}
			}

			if value, found := submap["source"]; found {
				strVal := fmt.Sprintf("%v", value)
				switch strVal {
				case "heartbeats":
					c.IdentityStatusConfig.Source = IdentityStatusSourceHeartbeats
				case "connect-events":
					c.IdentityStatusConfig.Source = IdentityStatusSourceConnectEvents
				case "hybrid":
					c.IdentityStatusConfig.Source = IdentityStatusSourceHybrid
				default:
					pfxlog.Logger().Errorf("invalid value '%v' for identity status config source, valid values: ['heartbeats', 'connect-events', 'hybrid']", strVal)
				}
			}

		} else {
			return errors.Errorf("invalid type for identityStatusConfig, should be map instead of %T", value)
		}
	}

	return nil
}

func LoadEdgeConfigFromMap(configMap map[interface{}]interface{}) (*EdgeConfig, error) {
	edgeConfig := NewEdgeConfig()

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

	if err = edgeConfig.loadTotpSection(edgeConfigMap); err != nil {
		return nil, err
	}

	if err = edgeConfig.loadEnrollmentSection(edgeConfigMap); err != nil {
		return nil, err
	}

	if err = edgeConfig.loadAuthRateLimiterConfig(edgeConfigMap); err != nil {
		return nil, err
	}

	if err = edgeConfig.loadIdentityStatusConfig(edgeConfigMap); err != nil {
		return nil, err
	}

	return edgeConfig, nil
}

// CalculateCaPems takes the supplied caPems buffer as a set of PEM Certificates separated by new lines. Duplicate
// certificates are removed, and the result is returned as a bytes.Buffer of PEM Certificates separated by new lines.
func CalculateCaPems(caPems *bytes.Buffer) *bytes.Buffer {
	caPemMap := map[string][]byte{}

	newCaPems := bytes.Buffer{}
	blocksToProcess := caPems.Bytes()

	for len(blocksToProcess) != 0 {
		var block *pem.Block
		block, blocksToProcess = pem.Decode(blocksToProcess)

		if block != nil {

			if block.Type != "CERTIFICATE" {
				pfxlog.Logger().
					WithField("type", block.Type).
					WithField("block", string(pem.EncodeToMemory(block))).
					Warn("encountered an invalid PEM block type loading configured CAs, block will be ignored")
				continue
			}

			cert, err := x509.ParseCertificate(block.Bytes)

			if err != nil {
				pfxlog.Logger().
					WithField("type", block.Type).
					WithField("block", string(pem.EncodeToMemory(block))).
					WithError(err).
					Warn("block could not be parsed as a certificate, block will be ignored")
				continue
			}

			if !cert.IsCA {
				pfxlog.Logger().
					WithField("type", block.Type).
					WithField("block", string(pem.EncodeToMemory(block))).
					Warn("block is not a CA, block will be ignored")
				continue
			}
			// #nosec
			hash := sha1.Sum(block.Bytes)
			fingerprint := toHex(hash[:])
			newPem := pem.EncodeToMemory(block)
			caPemMap[fingerprint] = newPem
		} else {
			blocksToProcess = nil
		}
	}

	for _, caPem := range caPemMap {
		_, _ = newCaPems.WriteString("\n")
		_, _ = newCaPems.Write(caPem)
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
