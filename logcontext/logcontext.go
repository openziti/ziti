package logcontext

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/sirupsen/logrus"
)

const (
	SelectPath        = "selectPath"
	EstablishPath     = "establishPath"
	MaskSelectPath    = uint32(1)
	MaskEstablishPath = uint32(2)
)

var channelMap = map[string]uint32{}

func init() {
	channelMap[SelectPath] = MaskSelectPath
	channelMap[EstablishPath] = MaskEstablishPath
}

func GetChannelMask(s string) uint32 {
	return channelMap[s]
}

func getDebugLogger() *logrus.Logger {
	return pfxlog.LevelLogger(logrus.DebugLevel)
}

type Context interface {
	pfxlog.Wirer
	SetChannelsMask(s uint32)
	GetChannelsMask() uint32
	GetFields() map[string]interface{}
	GetStringFields() map[string]string
	WithFields(fields map[string]interface{}) Context
	WithField(field string, value interface{}) Context
}

func NewContext() Context {
	return &contextImpl{
		fields: map[string]interface{}{},
	}
}

func NewContextWith(channelMask uint32, fields map[string]string) Context {
	result := &contextImpl{
		channels: channelMask,
		fields:   map[string]interface{}{},
	}

	for k, v := range fields {
		result.fields[k] = v
	}

	return result
}

type contextImpl struct {
	channels uint32
	fields   map[string]interface{}
}

func (self *contextImpl) WireEntry(entry *logrus.Entry) *logrus.Entry {
	if self != nil {
		if len(self.fields) > 0 {
			entry = entry.WithFields(self.fields)
		}
		if self.channels != 0 && entry.Level < logrus.DebugLevel {
			if val, found := entry.Data["channels"]; found {
				if channels, ok := val.([]string); ok {
					for _, channel := range channels {
						s := channelMap[channel]
						if s&self.channels != 0 {
							entry.Logger = getDebugLogger()
							break
						}
					}
				}
			}
		}
	}
	return entry
}

func (self *contextImpl) GetChannelsMask() uint32 {
	return self.channels
}

func (self *contextImpl) SetChannelsMask(s uint32) {
	self.channels = s
}

func (self *contextImpl) GetFields() map[string]interface{} {
	return self.fields
}

func (self *contextImpl) GetStringFields() map[string]string {
	result := map[string]string{}
	for k, v := range self.fields {
		if s, ok := v.(string); ok {
			result[k] = s
		} else {
			result[k] = fmt.Sprintf("%v", v)
		}
	}
	return result
}

func (self *contextImpl) WithFields(fields map[string]interface{}) Context {
	for k, v := range fields {
		self.fields[k] = v
	}
	return self
}

func (self *contextImpl) WithField(k string, v interface{}) Context {
	self.fields[k] = v
	return self
}
