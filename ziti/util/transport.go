// Package util provides utility functions for the Ziti CLI, including HTTP transport
// creation and configuration for communicating over Ziti networks.
package util

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/ziti/constants"
)

// NewZitifiedTransportFromSlice creates an HTTP transport configured to route
// connections through a Ziti network. The provided bytes should contain a JSON-encoded
// Ziti configuration.
//
// By default, urls are expected to leverage intercepts. Create a service and assign an appropriate
// intercept config and use the intercept address when dialing.
//
// To support addressable terminators-based dialing a user should be specified in the URL. This activates
// the dial-by-identity functionality. In this mode the url should be in the form of
// "identity-to-dial@service-name-to-dial". The transport uses the Proxy hook to extract user identity
// information from request URLs and passes it to Ziti dial operation via DialOptions.
//
// Returns an error if the configuration is invalid or Ziti context creation fails.
func NewZitifiedTransportFromSlice(bytes []byte, terminator string) (*http.Transport, error) {
	ztx, cerr := NewZitifiedContextFromSlice(bytes)
	if cerr != nil {
		return nil, cerr
	}

	zitiTransport := http.DefaultTransport.(*http.Transport).Clone()

	opts := ziti.DialOptions{
		Identity: terminator,
	}
	zitiTransport.DialContext = NewZitiDialContext(ztx, opts)

	return zitiTransport, nil
}

// ZitifiedTransportFromEnv creates a Ziti-enabled HTTP transport by reading a
// base64-encoded Ziti identity from the default environment variable
// (ZitiCliNetworkIdVarName from constants).
//
// Returns (nil, nil) if the environment variable is not set, or (transport, error)
// if there's an issue creating the transport.
func ZitifiedTransportFromEnv(terminator string) (*http.Transport, error) {
	return ZitifiedTransportFromEnvByName(constants.ZitiCliNetworkIdVarName, terminator)
}

// ZitifiedTransportFromEnvByName creates a Ziti-enabled HTTP transport by reading
// a base64-encoded Ziti identity from the specified environment variable.
//
// The environment variable should contain a base64-encoded Ziti configuration.
// Returns (nil, nil) if the environment variable is not set, or (transport, error)
// if there are issues with decoding or configuration creation.
func ZitifiedTransportFromEnvByName(envVarName string, terminator string) (*http.Transport, error) {
	data, err := ZitiConfigFromEnvByName(envVarName)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return NewZitifiedTransportFromSlice(data, terminator)
}

// NewZitifiedTransportFromFile creates a Ziti-enabled HTTP transport by reading
// a Ziti configuration from a file. The file should contain JSON-encoded Ziti
// configuration data.
//
// Returns an error if the file cannot be read or contains invalid configuration.
func NewZitifiedTransportFromFile(pathToFile string, terminator string) (*http.Transport, error) {
	data, err := os.ReadFile(pathToFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read ziti identity file %s: %v", pathToFile, err)
	}
	return NewZitifiedTransportFromSlice(data, terminator)
}

// NewZitiDialContext creates a dial context function that routes connections through
// a Ziti network. The returned function can be used as the DialContext for http.Transport.
//
// If opts.Identity is specified, the function performs addressable terminator-based dialing
// by extracting the hostname from the address and passing it to Ziti. Otherwise, it uses
// the fallback dialer from the context collection.
func NewZitiDialContext(zc ziti.Context, opts ziti.DialOptions) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := zitiCliContextCollection.NewDialerWithFallback(ctx, &net.Dialer{})
		if opts.Identity != "" {
			hostParts := strings.Split(addr, ":")
			return zc.DialWithOptions(hostParts[0], &opts)
		} else {
			return dialer.Dial(network, addr)
		}
	}
}

// NewZitifiedContextFromSlice creates a Ziti context from JSON-encoded configuration bytes.
// The context is configured to retrieve all config types and is added to the global context
// collection. Services are loaded and validated before returning.
//
// Returns an error if the configuration is invalid, context creation fails, or services
// cannot be retrieved.
func NewZitifiedContextFromSlice(bytes []byte) (ziti.Context, error) {
	if len(bytes) == 0 {
		return nil, nil
	}
	cfg := &ziti.Config{}
	if err := json.Unmarshal(bytes, &cfg); err != nil {
		return nil, err
	}
	cfg.ConfigTypes = append(cfg.ConfigTypes, "all")

	zc, zce := ziti.NewContext(cfg)
	if zce != nil {
		return nil, fmt.Errorf("failed to create ziti context: %v", zce)
	}
	zitiCliContextCollection.Add(zc)

	if _, se := zc.GetServices(); se != nil {
		return nil, fmt.Errorf("failed to get ziti services: %v", se)
	}

	_, se := zc.GetServices() // loads all the services
	if se != nil {
		return nil, fmt.Errorf("failed to get ziti services: %v", se)
	}

	return zc, nil
}

// ZitiConfigFromEnv reads a base64-encoded Ziti configuration from the default
// environment variable (ZitiCliNetworkIdVarName from constants).
//
// Returns (nil, nil) if the environment variable is not set, or (config, error)
// if there are issues with decoding.
func ZitiConfigFromEnv() ([]byte, error) {
	return ZitiConfigFromEnvByName(constants.ZitiCliNetworkIdVarName)
}

// ZitiConfigFromEnvByName reads a base64-encoded Ziti configuration from the specified
// environment variable.
//
// Returns (nil, nil) if the environment variable is not set, or (config, error) if there
// are issues decoding the base64-encoded configuration.
func ZitiConfigFromEnvByName(envVarName string) ([]byte, error) {
	b64Zid := os.Getenv(envVarName)
	if b64Zid == "" {
		return nil, nil
	}
	idReader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(b64Zid))
	data, err := io.ReadAll(idReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read and decode ziti identity: %v", err)
	}
	return data, nil
}
