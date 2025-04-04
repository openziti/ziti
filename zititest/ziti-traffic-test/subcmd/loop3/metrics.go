package loop3

import (
	"encoding/binary"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/pkg/errors"
	"github.com/rcrowley/go-metrics"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

var registry = metrics.NewRegistry()

var ConnectionTime = metrics.NewTimer()
var MsgTxRate = metrics.NewMeter()
var MsgRxRate = metrics.NewMeter()
var BytesTxRate = metrics.NewMeter()
var BytesRxRate = metrics.NewMeter()
var MsgLatency = metrics.NewTimer()

func init() {
	register := func(name string, metric interface{}) {
		if err := registry.Register(name, metric); err != nil {
			panic(err)
		}
	}

	register("tx.msg.rate", MsgTxRate)
	register("tx.bytes.rate", BytesTxRate)
	register("rx.msg.rate", MsgRxRate)
	register("rx.bytes.rate", BytesRxRate)
	register("msg.latency", MsgLatency)
	register("conn.time", ConnectionTime)
}

func StartMetricsReporter(configFile string, metrics *Metrics, closer chan struct{}) error {
	if metrics.ReportInterval == 0 {
		return errors.New("metrics report interval must be greater than 0")
	}

	reporter := &zitiMetricsReporter{
		configFile: configFile,
		clientId:   metrics.ClientId,
		closer:     closer,
		service:    metrics.Service,
	}

	go reporter.run(metrics.ReportInterval)
	return nil
}

type zitiMetricsReporter struct {
	configFile string
	clientId   string
	closer     chan struct{}
	service    string
}

func (r *zitiMetricsReporter) run(reportInterval time.Duration) {
	log := pfxlog.Logger()
	log.Infof("reporting metrics every %v", reportInterval)

	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()

	var client ziti.Context
	if r.configFile != "" {
		sdkConfig, err := ziti.NewConfigFromFile(r.configFile)
		if err != nil {
			panic(err)
		}
		client, err = ziti.NewContext(sdkConfig)

		if err != nil {
			panic(err)
		}
	} else {
		panic("no configuration file provided")
	}
	conn, err := client.Dial(r.service)

	if err != nil {
		panic(err)
	}

	defer func() { _ = conn.Close() }()

	lenBuf := make([]byte, 4)

	for {
		select {
		case <-ticker.C:
			event := r.createMetricsEvent()
			buf, err := proto.Marshal(event)
			if err != nil {
				log.WithError(err).Error("unable to marshal streaming metrics")
			} else {
				length := len(buf)
				binary.LittleEndian.PutUint32(lenBuf, uint32(length))
				log.Infof("sending metrics message with len %v, %+v", length, lenBuf)
				if _, err := conn.Write(lenBuf); err != nil {
					panic(err)
				}
				if _, err := conn.Write(buf); err != nil {
					panic(err)
				}
				log.Info("reported metrics")
			}
		case <-r.closer:
			log.Info("stopping metrics reporter")
			return
		}
	}
}

func (r *zitiMetricsReporter) createMetricsEvent() *mgmt_pb.StreamMetricsEvent {
	event := &mgmt_pb.StreamMetricsEvent{
		Timestamp:    timestamppb.Now(),
		SourceId:     r.clientId,
		IntMetrics:   map[string]int64{},
		FloatMetrics: map[string]float64{},
		MetricGroup:  map[string]string{},
	}

	registry.Each(func(name string, i interface{}) {
		switch metric := i.(type) {
		case metrics.Gauge:
			r.addIntMetric(event, name, "value", metric.Value())
		case metrics.Meter:
			r.addMeterToEvent(event, name, metric)
		case metrics.Histogram:
			r.addHistogramToEvent(event, name, metric)
		case metrics.Timer:
			r.addTimerToEvent(event, name, metric)
		}
	})

	return event
}

func (r *zitiMetricsReporter) addMeterToEvent(event *mgmt_pb.StreamMetricsEvent, name string, metric metrics.Meter) {
	r.addIntMetric(event, name, "count", metric.Count())
	r.addFloatMetric(event, name, "mean_rate", metric.RateMean())
	r.addFloatMetric(event, name, "m1_rate", metric.Rate1())
	r.addFloatMetric(event, name, "m5_rate", metric.Rate5())
	r.addFloatMetric(event, name, "m15_rate", metric.Rate15())
}

func (r *zitiMetricsReporter) addHistogramToEvent(event *mgmt_pb.StreamMetricsEvent, name string, metric metrics.Histogram) {
	r.addIntMetric(event, name, "count", metric.Count())
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

func (r *zitiMetricsReporter) addTimerToEvent(event *mgmt_pb.StreamMetricsEvent, name string, metric metrics.Timer) {
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

func (r *zitiMetricsReporter) addIntMetric(event *mgmt_pb.StreamMetricsEvent, base, name string, val int64) {
	fullName := base + "." + name
	event.IntMetrics[fullName] = val
	event.MetricGroup[fullName] = base
}

func (r *zitiMetricsReporter) addFloatMetric(event *mgmt_pb.StreamMetricsEvent, base, name string, val float64) {
	fullName := base + "." + name
	event.FloatMetrics[fullName] = val
	event.MetricGroup[fullName] = base
}
