/*
	Copyright 2019 Netfoundry, Inc.

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

package metrics

import (
	"github.com/netfoundry/ziti-fabric/fabric/pb/ctrl_pb"
	"errors"
	"fmt"
	"github.com/golang/protobuf/ptypes"
	influxdb "github.com/influxdata/influxdb1-client"
	"github.com/michaelquigley/pfxlog"
	"net/url"
	"time"
)

type influxReporter struct {
	url         url.URL
	database    string
	username    string
	password    string
	metricsChan chan *ctrl_pb.MetricsMessage

	client *influxdb.Client
}

func (reporter *influxReporter) AcceptMetrics(message *ctrl_pb.MetricsMessage) {
	reporter.metricsChan <- message
}

// NewInfluxDBMetricsHandler creates a new HandlerTypeInfluxDB metrics ChannelReporter
func NewInfluxDBMetricsHandler(cfg *influxConfig) (Handler, error) {
	rep := &influxReporter{
		url:         cfg.url,
		database:    cfg.database,
		username:    cfg.username,
		password:    cfg.password,
		metricsChan: make(chan *ctrl_pb.MetricsMessage, 10),
	}

	if err := rep.makeClient(); err != nil {
		return nil, fmt.Errorf("unable to make HandlerTypeInfluxDB influxdb. err=%v", err)
	}

	go rep.run()
	return rep, nil
}

func (reporter *influxReporter) makeClient() (err error) {
	reporter.client, err = influxdb.NewClient(influxdb.Config{
		URL:      reporter.url,
		Username: reporter.username,
		Password: reporter.password,
	})

	return
}

func (reporter *influxReporter) run() {
	log := pfxlog.Logger()
	log.Info("started")
	defer log.Warn("exited")

	pingTicker := time.Tick(time.Second * 5)

	for {
		select {
		case msg := <-reporter.metricsChan:
			if err := reporter.send(msg); err != nil {
				log.Printf("unable to send metrics to HandlerTypeInfluxDB. err=%v", err)
			}
		case <-pingTicker:
			_, _, err := reporter.client.Ping()
			if err != nil {
				log.Printf("got error while sending a ping to HandlerTypeInfluxDB, trying to recreate influxdb. err=%v", err)

				if err = reporter.makeClient(); err != nil {
					log.Printf("unable to make HandlerTypeInfluxDB influxdb. err=%v", err)
				}
			}
		}
	}
}

func (reporter *influxReporter) send(msg *ctrl_pb.MetricsMessage) error {
	var pts []influxdb.Point

	now, err := ptypes.Timestamp(msg.Timestamp)

	if err != nil {
		return err
	}

	tags := make(map[string]string)
	for k, v := range msg.Tags {
		tags[k] = v
	}
	tags["source"] = msg.SourceId

	for name, val := range msg.IntValues {
		pts = append(pts, influxdb.Point{
			Measurement: name,
			Tags:        tags,
			Fields: map[string]interface{}{
				"value": val,
			},
			Time: now,
		})
	}

	for name, val := range msg.FloatValues {
		pts = append(pts, influxdb.Point{
			Measurement: name,
			Tags:        tags,
			Fields: map[string]interface{}{
				"value": val,
			},
			Time: now,
		})
	}

	for name, val := range msg.Histograms {
		pts = append(pts, influxdb.Point{
			Measurement: name,
			Tags:        tags,
			Fields: map[string]interface{}{
				"count":    val.Count,
				"max":      val.Max,
				"mean":     val.Mean,
				"min":      val.Min,
				"stddev":   val.StdDev,
				"variance": val.Variance,
				"p50":      val.P50,
				"p75":      val.P75,
				"p95":      val.P95,
				"p99":      val.P99,
				"p999":     val.P999,
				"p9999":    val.P9999,
			},
			Time: now,
		})
	}

	for name, val := range msg.Meters {
		pts = append(pts, influxdb.Point{
			Measurement: name,
			Tags:        tags,
			Fields: map[string]interface{}{
				"count": val.Count,
				"m1":    val.M1Rate,
				"m5":    val.M5Rate,
				"m15":   val.M15Rate,
				"mean":  val.MeanRate,
			},
			Time: now,
		})
	}

	bps := influxdb.BatchPoints{
		Points:   pts,
		Database: reporter.database,
	}

	_, err = reporter.client.Write(bps)
	return err
}

type influxConfig struct {
	url      url.URL
	database string
	username string
	password string
}

func LoadInfluxConfig(src map[interface{}]interface{}) (*influxConfig, error) {
	cfg := &influxConfig{}

	if value, found := src["url"]; found {
		if urlSrc, ok := value.(string); ok {
			if url, err := url.Parse(urlSrc); err == nil {
				cfg.url = *url
			} else {
				return nil, fmt.Errorf("cannot parse influx 'url' value (%s)", err)
			}
		} else {
			return nil, errors.New("invalid influx 'url' value")
		}
	} else {
		return nil, errors.New("missing influx 'url' config")
	}

	if value, found := src["database"]; found {
		if database, ok := value.(string); ok {
			cfg.database = database
		} else {
			return nil, errors.New("invalid influx 'database' value")
		}
	} else {
		return nil, errors.New("missing influx 'database' config")
	}

	if value, found := src["username"]; found {
		if username, ok := value.(string); ok {
			cfg.username = username
		} else {
			return nil, errors.New("invalid influx 'username' value")
		}
	}

	if value, found := src["password"]; found {
		if password, ok := value.(string); ok {
			cfg.password = password
		} else {
			return nil, errors.New("invalid influx 'password' value")
		}
	}

	return cfg, nil
}
