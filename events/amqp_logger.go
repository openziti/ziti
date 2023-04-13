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

	"github.com/pkg/errors"
	amqp "github.com/rabbitmq/amqp091-go"
)

type AMQPEventLoggerFactory struct{}

func (AMQPEventLoggerFactory) NewEventHandler(config map[interface{}]interface{}) (interface{}, error) {
	return NewAMQPEventLogger(fabricFormatterFactory{}, config)
}

type amqpWriteCloser struct {
	queue amqp.Queue
	ch    *amqp.Channel
	conn  *amqp.Connection
}

func (wc amqpWriteCloser) Write(data []byte) (int, error) {
	err := wc.ch.PublishWithContext(context.Background(), "", wc.queue.Name, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        data,
	})
	if err != nil {
		return 0, err
	}
	return len(data), err
}

func (wc amqpWriteCloser) Close() error {
	var retErr error
	//TODO: Update to errors.Join on go 1.20 update
	if wc.ch != nil {
		if err := wc.ch.Close(); err != nil {
			retErr = err
		}
	}
	if wc.conn != nil {
		if err := wc.conn.Close(); err != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v; %w", retErr, err)
			} else {
				retErr = err
			}
		}
	}
	return retErr
}

type amqpConfig struct {
	url        string
	queueName  string
	durable    bool
	autoDelete bool
	exclusive  bool
	noWait     bool
}

func parseAMQPConfig(config map[interface{}]interface{}) (*amqpConfig, error) {
	ret := &amqpConfig{
		durable: true,
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

	conn, err := amqp.Dial(conf.url)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to dial amqp server at %s", conf.url)
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, errors.Wrap(err, "error getting amqp channel")
	}

	queue, err := ch.QueueDeclare(conf.queueName, conf.durable, conf.autoDelete, conf.exclusive, conf.noWait, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error declaring queue")
	}

	if value, found := config["format"]; found {
		if format, ok := value.(string); ok {
			return formatterFactory.NewLoggingHandler(format, bufferSize, amqpWriteCloser{
				queue: queue,
				ch:    ch,
				conn:  conn,
			})
		}
		return nil, errors.New("invalid 'format' for event amqp log")

	}
	return nil, errors.New("'format' must be specified for event handler")
}
