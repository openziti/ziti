package metrics

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	gometrics "github.com/rcrowley/go-metrics"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

type ZitiReporter struct {
	registry metrics.Registry
	client   ziti.Context
	clientId string
	closer   chan struct{}
	service  string
}

type ZitiReporterConfig struct {
	Registry           metrics.Registry
	Client             ziti.Context
	IdentityConfigFile string
	ClientId           string
	CloseNotify        chan struct{}
	ServiceName        string
}

func NewZitiReporter(cfg *ZitiReporterConfig) (*ZitiReporter, error) {
	if cfg.Registry == nil {
		return nil, errors.New("missing metrics registry for ziti metrics reporter")
	}

	if cfg.ClientId == "" {
		return nil, errors.New("missing client id for ziti metrics reporter")
	}

	if cfg.ServiceName == "" {
		return nil, errors.New("missing service name for ziti metrics reporter")
	}

	client := cfg.Client
	if client == nil {
		if cfg.IdentityConfigFile != "" {
			sdkConfig, err := ziti.NewConfigFromFile(cfg.IdentityConfigFile)
			if err != nil {
				return nil, fmt.Errorf("unable to load ziti identity for metrics reporter (%w)", err)
			}
			client, err = ziti.NewContext(sdkConfig)
			if err != nil {
				return nil, fmt.Errorf("unable to create ziti sdk client for metrics reporter (%w)", err)
			}
		} else {
			return nil, errors.New("no ziti client and no config file provided for metris reporter")
		}
	}

	return &ZitiReporter{
		registry: cfg.Registry,
		client:   client,
		clientId: cfg.ClientId,
		closer:   cfg.CloseNotify,
		service:  cfg.ServiceName,
	}, nil
}

func (r *ZitiReporter) Run(reportInterval time.Duration) {
	log := pfxlog.Logger().WithField("clientId", r.client).WithField("service", r.service)
	log.Infof("reporting metrics every %v", reportInterval)

	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()

	var err error
	var conn edge.Conn

	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
	}()

	lenBuf := make([]byte, 4)

	for {
		select {
		case <-ticker.C:
			if conn == nil {
				conn, err = r.client.Dial(r.service)
				if err != nil {
					log.WithError(err).Error("failed to dial metrics services")
					continue
				}
			}

			event := r.createMetricsEvent()
			buf, err := proto.Marshal(event)
			if err != nil {
				log.WithError(err).Error("unable to marshal streaming metrics")
			} else {
				length := len(buf)
				binary.LittleEndian.PutUint32(lenBuf, uint32(length))
				log.Infof("sending metrics message with len %v, %+v", length, lenBuf)
				if _, err = conn.Write(lenBuf); err != nil {
					log.WithError(err).Error("failed to write metrics message length")
					_ = conn.Close()
					conn = nil
				} else if _, err = conn.Write(buf); err != nil {
					log.WithError(err).Error("failed to write metrics message length")
					_ = conn.Close()
					conn = nil
				} else {
					log.Info("reported metrics")
				}
			}
		case <-r.closer:
			log.Info("stopping metrics reporter")
			return
		}
	}
}

func (r *ZitiReporter) createMetricsEvent() *mgmt_pb.StreamMetricsEvent {
	event := &mgmt_pb.StreamMetricsEvent{
		Timestamp:    timestamppb.Now(),
		SourceId:     r.clientId,
		IntMetrics:   map[string]int64{},
		FloatMetrics: map[string]float64{},
		MetricGroup:  map[string]string{},
	}

	r.registry.AcceptVisitor(&visitor{
		event: event,
	})

	return event
}

func (r *ZitiReporter) addTimerToEvent(event *mgmt_pb.StreamMetricsEvent, name string, metric gometrics.Timer) {
	r.addIntMetric(event, name, "count", metric.Count())
	r.addFloatMetric(event, name, "mean_rate", metric.RateMean())
	r.addFloatMetric(event, name, "m1_rate", metric.Rate1())
	r.addFloatMetric(event, name, "m5_rate", metric.Rate5())
	r.addFloatMetric(event, name, "m15_rate", metric.Rate15())
	r.addIntMetric(event, name, "min", metric.Min())
	r.addIntMetric(event, name, "max", metric.Max())
	r.addFloatMetric(event, name, "mean", metric.Mean())
	r.addFloatMetric(event, name, "std_dev", metric.StdDev())
	r.addFloatMetric(event, name, "variance", metric.Variance())
	r.addFloatMetric(event, name, "p50", metric.Percentile(.50))
	r.addFloatMetric(event, name, "p75", metric.Percentile(.75))
	r.addFloatMetric(event, name, "p95", metric.Percentile(.95))
	r.addFloatMetric(event, name, "p99", metric.Percentile(.99))
	r.addFloatMetric(event, name, "p999", metric.Percentile(.999))
	r.addFloatMetric(event, name, "p9999", metric.Percentile(.9999))
}

func (r *ZitiReporter) addIntMetric(event *mgmt_pb.StreamMetricsEvent, base, name string, val int64) {
	fullName := base + "." + name
	event.IntMetrics[fullName] = val
	event.MetricGroup[fullName] = base
}

func (r *ZitiReporter) addFloatMetric(event *mgmt_pb.StreamMetricsEvent, base, name string, val float64) {
	fullName := base + "." + name
	event.FloatMetrics[fullName] = val
	event.MetricGroup[fullName] = base
}

type visitor struct {
	event *mgmt_pb.StreamMetricsEvent
}

func (self *visitor) addIntMetric(base, name string, val int64) {
	fullName := base + "." + name
	self.event.IntMetrics[fullName] = val
	self.event.MetricGroup[fullName] = base
}

func (self *visitor) addFloatMetric(base, name string, val float64) {
	fullName := base + "." + name
	self.event.FloatMetrics[fullName] = val
	self.event.MetricGroup[fullName] = base
}

func (self *visitor) VisitGauge(name string, metric metrics.Gauge) {
	self.addIntMetric(name, "value", metric.Value())
}

func (self *visitor) VisitMeter(name string, metric metrics.Meter) {
	self.addIntMetric(name, "count", metric.Count())
	self.addFloatMetric(name, "mean_rate", metric.RateMean())
	self.addFloatMetric(name, "m1_rate", metric.Rate1())
	self.addFloatMetric(name, "m5_rate", metric.Rate5())
	self.addFloatMetric(name, "m15_rate", metric.Rate15())
}

func (self *visitor) VisitHistogram(name string, metric metrics.Histogram) {
	self.addIntMetric(name, "count", metric.Count())
	self.addIntMetric(name, "min", metric.Min())
	self.addIntMetric(name, "max", metric.Max())
	self.addFloatMetric(name, "mean", metric.Mean())
	self.addFloatMetric(name, "std_dev", metric.StdDev())
	self.addFloatMetric(name, "variance", metric.Variance())
	self.addFloatMetric(name, "p50", metric.Percentile(.50))
	self.addFloatMetric(name, "p75", metric.Percentile(.75))
	self.addFloatMetric(name, "p95", metric.Percentile(.95))
	self.addFloatMetric(name, "p99", metric.Percentile(.99))
	self.addFloatMetric(name, "p999", metric.Percentile(.999))
	self.addFloatMetric(name, "p9999", metric.Percentile(.9999))
}

func (self *visitor) VisitTimer(name string, metric metrics.Timer) {
	self.addIntMetric(name, "count", metric.Count())
	self.addFloatMetric(name, "mean_rate", metric.RateMean())
	self.addFloatMetric(name, "m1_rate", metric.Rate1())
	self.addFloatMetric(name, "m5_rate", metric.Rate5())
	self.addFloatMetric(name, "m15_rate", metric.Rate15())
	self.addIntMetric(name, "min", metric.Min())
	self.addIntMetric(name, "max", metric.Max())
	self.addFloatMetric(name, "mean", metric.Mean())
	self.addFloatMetric(name, "std_dev", metric.StdDev())
	self.addFloatMetric(name, "variance", metric.Variance())
	self.addFloatMetric(name, "p50", metric.Percentile(.50))
	self.addFloatMetric(name, "p75", metric.Percentile(.75))
	self.addFloatMetric(name, "p95", metric.Percentile(.95))
	self.addFloatMetric(name, "p99", metric.Percentile(.99))
	self.addFloatMetric(name, "p999", metric.Percentile(.999))
	self.addFloatMetric(name, "p9999", metric.Percentile(.9999))
}
