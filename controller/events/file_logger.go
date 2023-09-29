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
	"fmt"
	"github.com/natefinch/lumberjack"
	"github.com/pkg/errors"
	"io"
	"os"
	"strings"
)

type fabricFormatterFactory struct{}

func (f fabricFormatterFactory) NewLoggingHandler(format string, buffer int, out io.WriteCloser) (interface{}, error) {
	if strings.EqualFold(format, "json") {
		return NewJsonFormatter(buffer, NewWriterEventSink(out)), nil
	}

	return nil, errors.Errorf("invalid 'format' for event log output file: %v", format)
}

type StdOutLoggerFactory struct{}

func (StdOutLoggerFactory) NewEventHandler(config map[interface{}]interface{}) (interface{}, error) {
	return NewFileEventLogger(fabricFormatterFactory{}, true, config)
}

type FileEventLoggerFactory struct{}

func (FileEventLoggerFactory) NewEventHandler(config map[interface{}]interface{}) (interface{}, error) {
	return NewFileEventLogger(fabricFormatterFactory{}, false, config)
}

func NewFileEventLogger(formatterFactory LoggingHandlerFactory, stdout bool, config map[interface{}]interface{}) (interface{}, error) {
	// allow config to increase the buffer size
	bufferSize := 10
	if value, found := config["bufferSize"]; found {
		if size, ok := value.(int); ok {
			bufferSize = size
		}
	}

	var output io.WriteCloser = os.Stdout

	if !stdout {
		// allow config to override the max file size
		maxsize := 10
		if value, found := config["maxsizemb"]; found {
			if maxsizemb, ok := value.(int); ok {
				maxsize = maxsizemb
			}
		}

		// allow config to override the max file size
		maxBackupFiles := 0
		if value, found := config["maxbackups"]; found {
			if maxbackups, ok := value.(int); ok {
				maxBackupFiles = maxbackups
			}
		}

		// set the path or die if not specified
		filepath := ""
		if value, found := config["path"]; found {
			if testpath, ok := value.(string); ok {
				f, err := os.OpenFile(testpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
				if err != nil {
					return nil, fmt.Errorf("cannot write to log file path: %s", testpath)
				} else {
					filepath = testpath
					_ = f.Close()
				}
			} else {
				return nil, errors.New("invalid event FileLogger 'path' value")
			}
		} else {
			return nil, errors.New("missing required 'path' config for events FileLogger handler")
		}

		output = &newlineWriter{
			out: &lumberjack.Logger{
				Filename:   filepath,
				MaxSize:    maxsize,
				MaxBackups: maxBackupFiles,
			},
		}
	}

	if value, found := config["format"]; found {
		if format, ok := value.(string); ok {
			return formatterFactory.NewLoggingHandler(format, bufferSize, output)
		}
		return nil, errors.New("invalid 'format' for event log output file")

	}
	return nil, errors.New("'format' must be specified for event handler")
}

type newlineWriter struct {
	out io.WriteCloser
}

func (self newlineWriter) Close() error {
	return self.out.Close()
}

func (self newlineWriter) Write(p []byte) (int, error) {
	n, err := self.out.Write(p)
	if err != nil {
		return n, err
	}
	m, err := self.out.Write([]byte("\n"))
	return n + m, err
}
