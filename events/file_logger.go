package events

import (
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/metrics/metrics_pb"
	"github.com/openziti/foundation/util/iomonad"
	"github.com/pkg/errors"
	"os"
)

func registerFileLoggerEventHandlerType(config map[interface{}]interface{}) (EventHandlerFactory, bool)  {

	logger := pfxlog.Logger()
	logger.Info("Registering event handler for fabric events")

	rep := &FabricHandler{
		config: config,
	}

	return rep, true
}

type FabricHandler struct {
	name   string
	config map[interface{}]interface{}
	eventsChan chan interface{}
	path string
	maxsizemb int
}

// Will work for all fabric session event types
type SessionMessage struct {
	Namespace string
	EventType string
	SessionId string
	ClientId string
	ServiceId string
	Circuit string
}


func (handler *FabricHandler) NewEventHandler(config map[interface{}]interface{}) (interface{}, error) {

	// allow config to increase the buffer size
	bufferSize := 10
	if value, found := config["bufferSize"]; found {
		if size, ok := value.(int); ok {
			bufferSize = size
		}
	}

	// allow config to override the max file size
	maxsize := 10
	if value, found := config["maxsizemb"]; found {
		if maxsizemb, ok := value.(int); ok {
			maxsize = maxsizemb
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

	fabHandler :=  &FabricHandler{
		name: "FabricHandler",
		config: config,
		path: filepath,
		maxsizemb: maxsize,
		eventsChan: make(chan interface{}, bufferSize),
	}

	go fabHandler.run()
	return fabHandler, nil

}

func (handler *FabricHandler) SessionCreated(sessionId *identity.TokenId, clientId *identity.TokenId, serviceId string, circuit *network.Circuit) {

	message := SessionMessage{
		Namespace: "fabric.sessions",
		EventType: "created",
		SessionId: sessionId.Token,
		ClientId: clientId.Token,
		ServiceId: serviceId,
		Circuit: circuit.String(),
	}

	handler.eventsChan <- message
}

func (handler *FabricHandler) SessionDeleted(sessionId *identity.TokenId, clientId *identity.TokenId) {

	message := SessionMessage{
		Namespace: "fabric.sessions",
		EventType: "deleted",
		SessionId: sessionId.Token,
		ClientId: clientId.Token,
	}

	handler.eventsChan <- message

}

func (handler *FabricHandler) CircuitUpdated(sessionId *identity.TokenId, circuit *network.Circuit) {

	message := SessionMessage{
		Namespace: "fabric.sessions",
		EventType: "circuitUpdated",
		SessionId: sessionId.Token,
		Circuit: circuit.String(),
	}

	handler.eventsChan <- message
}

func (handler *FabricHandler) AcceptMetrics(message *metrics_pb.MetricsMessage) {
	// Not currently implemented
	// Duplicates metric logging functionality that exists elsewhere

}


func( handler *FabricHandler) Handle(message interface{}) {

	logger := pfxlog.Logger()

	out, err := handler.getFileHandle(handler.path, handler.maxsizemb)

	if err != nil {
		logger.Errorf("Error getting the file handle: %v", err)
		return
	}

	w := iomonad.Wrap(out)
	// json format
	marshalled, err := json.Marshal(message)
	if err != nil {
		logger.Errorf("Error marshalling JSON: %v", err)
		return
	}

	bytes := w.Write(marshalled)
	w.Println("")
	logger.Infof("Wrote %v bytes to the file....", bytes)
	defer func() { _ = out.Close() }()

}



func( handler *FabricHandler) getFileHandle(path string, maxsize int) (*os.File, error) {
	if stat, err := os.Stat(path); err == nil {
		// get the size
		size := stat.Size()
		if size >= int64(maxsize*1024*1024) {
			if err := os.Truncate(path, 0); err != nil {
				pfxlog.Logger().WithError(err).Errorf("failure while trucating metrics log file %v to size %vM", path, maxsize)
			}
		}
	} else {
		pfxlog.Logger().WithError(err).Errorf("failure while statting metrics log file %v", path)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("failure while opening metrics log file %v", path)
		return nil, err
	}

	return f, nil

}


func (handler *FabricHandler) run() {
	logger := pfxlog.Logger()
	logger.Info("Fabric event handler started")
	defer logger.Warn("exited")

	for {
		select {
		case msg := <-handler.eventsChan:
			handler.Handle(msg)
		}
	}
}