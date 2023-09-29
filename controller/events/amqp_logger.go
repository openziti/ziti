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
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/pkg/errors"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

type AMQPEventLoggerFactory struct{}

func (AMQPEventLoggerFactory) NewEventHandler(config map[interface{}]interface{}) (interface{}, error) {
	return NewAMQPEventLogger(fabricFormatterFactory{}, config)
}

type amqpWriteCloser struct {
	queue    amqp.Queue
	ch       *amqp.Channel
	conn     *amqp.Connection
	config   *amqpConfig
	ctx      context.Context
	cancel   context.CancelFunc
	messages chan []byte
}

func newAMQPWriteCloser(config *amqpConfig) amqpWriteCloser {
	ctx, cancel := context.WithCancel(context.Background())
	ret := amqpWriteCloser{
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
					//TODO: Update to errors.Join on go 1.20 update
					if ret.ch != nil {
						if err := ret.ch.Close(); err != nil {
							retErr = err
						}
					}
					if ret.conn != nil {
						if err := ret.conn.Close(); err != nil {
							if retErr != nil {
								retErr = fmt.Errorf("%v; %w", retErr, err)
							} else {
								retErr = err
							}
						}
					}
					if retErr != nil {
						logrus.Errorf("error closing amqp connections: %v", retErr)
						return
					}
					logrus.Info("closed connection to amqp server")
				}
			}
		}
	}()

	return ret
}

func (wc *amqpWriteCloser) sendMessage(message []byte) {
	for {
		select {
		case <-wc.ctx.Done():
			return
		default:
		}
		err := wc.ch.PublishWithContext(context.Background(), "", wc.queue.Name, false, false, amqp.Publishing{
			ContentType: "application/json",
			Body:        message,
		})
		if err == nil {
			return
		}
		if errors.Is(err, amqp.ErrClosed) {
			logrus.Info("Need to attempt reconnect")
			wc.connect()
			continue
		}
		logrus.WithField("body", string(message)).Error("error sending message to amqp")
	}
}

func (wc *amqpWriteCloser) connect() {
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
		conn, err := amqp.Dial(wc.config.url)
		if err != nil {
			logrus.Errorf("unable to dial amqp server at %s: %v", wc.config.url, err)
			return err
		}
		wc.conn = conn
		ch, err := conn.Channel()
		if err != nil {
			logrus.Errorf("error getting amqp channel: %v", err)
			return err
		}
		wc.ch = ch
		queue, err := ch.QueueDeclare(wc.config.queueName, wc.config.durable, wc.config.autoDelete, wc.config.exclusive, wc.config.noWait, nil)
		if err != nil {
			logrus.Errorf("error declaring queue: %v", err)
			return err
		}
		wc.queue = queue
		return nil
	}
	if err := backoff.Retry(operation, expBackoff); err != nil {
		logrus.Errorf("amqp connection failed after exponential backoff: %v", err)
		return
	}
	logrus.Infof("connected to amqp server at: %s", wc.config.url)
}

func (wc amqpWriteCloser) Write(data []byte) (int, error) {
	select {
	case wc.messages <- data:
		return len(data), nil
	default:
		return 0, fmt.Errorf("amqp queue full. Message: %s", string(data))
	}
}

func (wc amqpWriteCloser) Close() error {
	wc.cancel()
	return nil
}

type amqpConfig struct {
	url        string
	queueName  string
	durable    bool
	autoDelete bool
	exclusive  bool
	noWait     bool
	bufferSize int
}

func parseAMQPConfig(config map[interface{}]interface{}) (*amqpConfig, error) {
	ret := &amqpConfig{
		durable:    true,
		bufferSize: 50,
	}
	if value, found := config["url"]; !found {
		return nil, fmt.Errorf("missing amqp url")
	} else {
		if u, ok := value.(string); ok {
			ret.url = u
		}
	}

	if value, found := config["queue"]; !found {
		return nil, fmt.Errorf("missing amqp queue name")
	} else {
		if u, ok := value.(string); ok {
			ret.queueName = u
		}
	}

	if value, found := config["durable"]; found {
		if u, ok := value.(bool); ok {
			ret.durable = u
		}
	}

	if value, found := config["autoDelete"]; found {
		if u, ok := value.(bool); ok {
			ret.autoDelete = u
		}
	}

	if value, found := config["exclusive"]; found {
		if u, ok := value.(bool); ok {
			ret.exclusive = u
		}
	}

	if value, found := config["noWait"]; found {
		if u, ok := value.(bool); ok {
			ret.noWait = u
		}
	}

	if value, found := config["bufferSize"]; found {
		if u, ok := value.(int); ok {
			ret.bufferSize = u
		}
	}

	return ret, nil
}

func NewAMQPEventLogger(formatterFactory LoggingHandlerFactory, config map[interface{}]interface{}) (interface{}, error) {
	bufferSize := 10
	if value, found := config["bufferSize"]; found {
		if size, ok := value.(int); ok {
			bufferSize = size
		}
	}

	conf, err := parseAMQPConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse amqp config")
	}

	if value, found := config["format"]; found {
		if format, ok := value.(string); ok {
			return formatterFactory.NewLoggingHandler(format, bufferSize, newAMQPWriteCloser(conf))
		}
		return nil, errors.New("invalid 'format' for event amqp log")

	}
	return nil, errors.New("'format' must be specified for event handler")
}
