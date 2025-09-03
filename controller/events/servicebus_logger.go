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

package events

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/cenkalti/backoff/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var contentType = "application/json"

type ServiceBusEventLoggerFactory struct{}

func (ServiceBusEventLoggerFactory) NewEventHandler(config map[interface{}]interface{}) (interface{}, error) {
	return NewServiceBusEventLogger(fabricFormatterFactory{}, config)
}

type serviceBusWriteCloser struct {
	client    *azservicebus.Client
	sender    *azservicebus.Sender
	config    *serviceBusConfig
	ctx       context.Context
	cancel    context.CancelFunc
	messages  chan []byte
}

func newServiceBusWriteCloser(config *serviceBusConfig) serviceBusWriteCloser {
	ctx, cancel := context.WithCancel(context.Background())
	ret := serviceBusWriteCloser{
		config:   config,
		ctx:      ctx,
		cancel:   cancel,
		messages: make(chan []byte, config.bufferSize),
	}
	go func() {
		ret.connect()
		for {
			select {
			case m := <-ret.messages:
				{
					ret.sendMessage(m)
				}
			case <-ret.ctx.Done():
				{
					var retErr error
					if ret.sender != nil {
						if err := ret.sender.Close(context.Background()); err != nil {
							retErr = err
						}
					}
					if ret.client != nil {
						if err := ret.client.Close(context.Background()); err != nil {
							if retErr != nil {
								retErr = fmt.Errorf("%v; %w", retErr, err)
							} else {
								retErr = err
							}
						}
					}
					if retErr != nil {
						logrus.Errorf("error closing service bus connections: %v", retErr)
						return
					}
					logrus.Info("closed connection to service bus")
				}
			}
		}
	}()

	return ret
}

func (wc *serviceBusWriteCloser) sendMessage(message []byte) {
	for {
		select {
		case <-wc.ctx.Done():
			return
		default:
		}
		
		sbMessage := &azservicebus.Message{
			ContentType: &contentType,
			Body: message,
		}
		
		err := wc.sender.SendMessage(context.Background(), sbMessage, nil)
		if err == nil {
			return
		}
		
		// Check if it's a connection error that requires reconnection
		if isConnectionError(err) {
			logrus.Info("Need to attempt reconnect to service bus")
			wc.connect()
			continue
		}
		
		// Enhanced error logging with more details
		logEntry := logrus.WithFields(logrus.Fields{
			"body":        string(message),
			"error":       err.Error(),
			"error_type":  fmt.Sprintf("%T", err),
		})
		
		// Add destination information if available
		if wc.config.topicName != "" {
			logEntry = logEntry.WithField("destination", "topic:"+wc.config.topicName)
		} else if wc.config.queueName != "" {
			logEntry = logEntry.WithField("destination", "queue:"+wc.config.queueName)
		}
		
		// Add response error details if available
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			logEntry = logEntry.WithFields(logrus.Fields{
				"status_code": respErr.StatusCode,
				"raw_response": respErr.RawResponse,
			})
		}
		
		logEntry.Error("error sending message to service bus")
	}
}

func isConnectionError(err error) bool {
	// Check for common connection-related errors
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		// Check for connection-related status codes
		switch respErr.StatusCode {
		case 401, 403, 404, 408, 500, 502, 503, 504:
			return true
		}
	}
	
	// Check for context errors
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	
	// Check for network-related errors
	// Note: azservicebus doesn't export ErrConnectionClosed, so we check for specific error types
	if err != nil {
		// Check if it's a connection-related error by examining the error message
		errMsg := err.Error()
		if strings.Contains(errMsg, "connection") || strings.Contains(errMsg, "network") || strings.Contains(errMsg, "timeout") {
			return true
		}
	}
	
	return false
}

func (wc *serviceBusWriteCloser) connect() {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 1 * time.Second
	expBackoff.MaxInterval = 5 * time.Minute
	expBackoff.MaxElapsedTime = 0

	operation := func() error {
		select {
		case <-wc.ctx.Done():
			return backoff.Permanent(nil)
		default:
		}
		
		// Create client options
		clientOptions := &azservicebus.ClientOptions{}
		
		// Create the client
		client, err := azservicebus.NewClientFromConnectionString(wc.config.connectionString, clientOptions)
		if err != nil {
			logrus.Errorf("unable to create service bus client: %v", err)
			return err
		}
		wc.client = client
		
		// Create sender - support both topic and queue
		var sender *azservicebus.Sender
		if wc.config.topicName != "" {
			sender, err = client.NewSender(wc.config.topicName, nil)
		} else if wc.config.queueName != "" {
			sender, err = client.NewSender(wc.config.queueName, nil)
		} else {
			return fmt.Errorf("either topic or queue must be specified")
		}
		if err != nil {
			logrus.Errorf("error creating service bus sender: %v", err)
			return err
		}
		wc.sender = sender
		
		return nil
	}
	
	if err := backoff.Retry(operation, expBackoff); err != nil {
		logrus.Errorf("service bus connection failed after exponential backoff: %v", err)
		return
	}
	
	// Log the connection success with appropriate destination
	if wc.config.topicName != "" {
		logrus.Infof("connected to service bus topic: %s", wc.config.topicName)
	} else {
		logrus.Infof("connected to service bus queue: %s", wc.config.queueName)
	}
}

func (wc serviceBusWriteCloser) Write(data []byte) (int, error) {
	select {
	case wc.messages <- data:
		return len(data), nil
	default:
		return 0, fmt.Errorf("service bus queue full. Message: %s", string(data))
	}
}

func (wc serviceBusWriteCloser) Close() error {
	wc.cancel()
	return nil
}

type serviceBusConfig struct {
	connectionString string
	topicName        string
	queueName        string
	bufferSize       int
}

func parseServiceBusConfig(config map[interface{}]interface{}) (*serviceBusConfig, error) {
	ret := &serviceBusConfig{
		bufferSize: 50,
	}
	
	if value, found := config["connectionString"]; !found {
		return nil, fmt.Errorf("missing service bus connection string")
	} else {
		if u, ok := value.(string); ok {
			ret.connectionString = u
		} else {
			return nil, fmt.Errorf("invalid service bus connection string")
		}
	}

	// Check for topic or queue configuration
	if value, found := config["topic"]; found {
		if u, ok := value.(string); ok {
			ret.topicName = u
		} else {
			return nil, fmt.Errorf("invalid service bus topic name")
		}
	} else if value, found := config["queue"]; found {
		if u, ok := value.(string); ok {
			ret.queueName = u
		} else {
			return nil, fmt.Errorf("invalid service bus queue name")
		}
	} else {
		return nil, fmt.Errorf("either topic or queue must be specified for service bus")
	}

	if value, found := config["bufferSize"]; found {
		if u, ok := value.(int); ok {
			ret.bufferSize = u
		}
	}

	return ret, nil
}

func NewServiceBusEventLogger(formatterFactory LoggingHandlerFactory, config map[interface{}]interface{}) (interface{}, error) {
	bufferSize := 10
	if value, found := config["bufferSize"]; found {
		if size, ok := value.(int); ok {
			bufferSize = size
		}
	}

	conf, err := parseServiceBusConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse service bus config")
	}

	if value, found := config["format"]; found {
		if format, ok := value.(string); ok {
			return formatterFactory.NewLoggingHandler(format, bufferSize, newServiceBusWriteCloser(conf))
		}
		return nil, errors.New("invalid 'format' for event service bus log")
	}
	return nil, errors.New("'format' must be specified for event handler")
}
