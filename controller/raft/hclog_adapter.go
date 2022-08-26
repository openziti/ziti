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

package raft

import (
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/michaelquigley/pfxlog"
	"github.com/sirupsen/logrus"
	"io"
	"log"
	"runtime"
	"strings"
	"sync"
)

func NewHcLogrusLogger() hclog.Logger {
	log := logrus.New()
	log.SetFormatter(pfxlog.Logger().Entry.Logger.Formatter)

	return &logger{
		log: logrus.NewEntry(log),
	}
}

type logger struct {
	log *logrus.Entry
	sync.Mutex
	name string
}

func (self *logger) Log(level hclog.Level, msg string, args ...interface{}) {
	switch level {
	case hclog.Trace:
		self.Trace(msg, args...)
	case hclog.Debug:
		self.Debug(msg, args...)
	case hclog.Info:
		self.Info(msg, args...)
	case hclog.Warn:
		self.Warn(msg, args...)
	case hclog.Error:
		self.Error(msg, args...)
	case hclog.Off:
	}
}

func (self *logger) ImpliedArgs() []interface{} {
	var fields []interface{}
	for k, v := range self.log.Data {
		fields = append(fields, k)
		fields = append(fields, v)
	}
	return fields
}

func (self *logger) Name() string {
	return self.name
}

func (self *logger) Trace(msg string, args ...interface{}) {
	self.logToLogrus(logrus.TraceLevel, msg, args...)
}

func (self *logger) Debug(msg string, args ...interface{}) {
	self.logToLogrus(logrus.DebugLevel, msg, args...)
}

func (self *logger) Info(msg string, args ...interface{}) {
	self.logToLogrus(logrus.InfoLevel, msg, args...)
}

func (self *logger) Warn(msg string, args ...interface{}) {
	self.logToLogrus(logrus.WarnLevel, msg, args...)
}

func (self *logger) Error(msg string, args ...interface{}) {
	self.logToLogrus(logrus.ErrorLevel, msg, args...)
}

func (self *logger) logToLogrus(level logrus.Level, msg string, args ...interface{}) {
	log := self.log
	if len(args) > 0 {
		log = self.LoggerWith(args)
	}
	frame := self.getCaller()
	log = log.WithField("file", frame.File).WithField("func", frame.Function)
	log.Log(level, self.name+msg)
}

func (self *logger) IsTrace() bool {
	return self.log.Logger.IsLevelEnabled(logrus.TraceLevel)
}

func (self *logger) IsDebug() bool {
	return self.log.Logger.IsLevelEnabled(logrus.DebugLevel)
}

func (self *logger) IsInfo() bool {
	return self.log.Logger.IsLevelEnabled(logrus.InfoLevel)
}

func (self *logger) IsWarn() bool {
	return self.log.Logger.IsLevelEnabled(logrus.WarnLevel)
}

func (self *logger) IsError() bool {
	return self.log.Logger.IsLevelEnabled(logrus.ErrorLevel)
}

func (self *logger) With(args ...interface{}) hclog.Logger {
	return &logger{
		log: self.LoggerWith(args),
	}
}

func (self *logger) LoggerWith(args []interface{}) *logrus.Entry {
	l := self.log
	ml := len(args)
	var key string
	for i := 0; i < ml-1; i += 2 {
		keyVal := args[i]
		if keyStr, ok := keyVal.(string); ok {
			key = keyStr
		} else {
			key = fmt.Sprintf("%v", keyVal)
		}
		val := args[i+1]
		if f, ok := val.(hclog.Format); ok {
			val = fmt.Sprintf(f[0].(string), f[1:])
		}
		l = l.WithField(key, val)
	}
	return l
}

func (self *logger) Named(name string) hclog.Logger {
	return self.ResetNamed(name + self.name)
}

func (self *logger) ResetNamed(name string) hclog.Logger {
	return &logger{
		name: name,
		log:  self.log,
	}
}

func (self *logger) SetLevel(level hclog.Level) {
	panic("implement me")
}

func (self *logger) StandardLogger(opts *hclog.StandardLoggerOptions) *log.Logger {
	panic("implement me")
}

func (self *logger) StandardWriter(opts *hclog.StandardLoggerOptions) io.Writer {
	panic("implement me")
}

var (
	// qualified package name, cached at first use
	localPackage string

	// Positions in the call stack when tracing to report the calling method
	minimumCallerDepth = 1

	// Used for caller information initialisation
	callerInitOnce sync.Once
)

const (
	maximumCallerDepth      int = 25
	knownLocalPackageFrames int = 4
)

// getCaller retrieves the name of the first non-logrus calling function
// derived from logrus code
func (self *logger) getCaller() *runtime.Frame {
	// cache this package's fully-qualified name
	callerInitOnce.Do(func() {
		pcs := make([]uintptr, maximumCallerDepth)
		_ = runtime.Callers(0, pcs)

		// dynamic get the package name and the minimum caller depth
		for i := 0; i < maximumCallerDepth; i++ {
			funcName := runtime.FuncForPC(pcs[i]).Name()
			if strings.Contains(funcName, "getCaller") {
				localPackage = self.getPackageName(funcName)
				// fmt.Printf("local package: %v\n", localPackage)
				break
			}
		}

		minimumCallerDepth = knownLocalPackageFrames
	})

	// Restrict the lookback frames to avoid runaway lookups
	pcs := make([]uintptr, maximumCallerDepth)
	depth := runtime.Callers(minimumCallerDepth, pcs)
	frames := runtime.CallersFrames(pcs[:depth])

	for f, again := frames.Next(); again; f, again = frames.Next() {
		pkg := self.getPackageName(f.Function)

		// If the caller isn't part of this package, we're done
		if pkg != localPackage {
			//fmt.Printf("frame func: %v\n", f.Function)
			return &f //nolint:scopelint
		}
	}

	// fmt.Printf("frame func not found\n")

	// if we got here, we failed to find the caller's context
	return nil
}

// derived from logrus code
// getPackageName reduces a fully qualified function name to the package name
// There really ought to be a better way...
func (self *logger) getPackageName(f string) string {
	for {
		lastPeriod := strings.LastIndex(f, ".")
		lastSlash := strings.LastIndex(f, "/")
		if lastPeriod > lastSlash {
			f = f[:lastPeriod]
		} else {
			break
		}
	}

	return f
}
