package main

import (
	"testing"

	"github.com/openziti/fablab/kernel/model"
	"github.com/stretchr/testify/require"
)

func Test_PopulateGaugeFloat64(t *testing.T) {
	req := require.New(t)

	metrics := &ServiceMetrics{}
	metrics.PopulateGaugeFloat64(&metrics.byteRate, model.MetricSet{"value": 1234.5})
	req.NoError(metrics.err)
	req.Equal(1234.5, *metrics.byteRate)

	metrics = &ServiceMetrics{}
	metrics.PopulateGaugeFloat64(&metrics.byteRate, model.MetricSet{})
	req.Error(metrics.err, "a gauge with no value is reported")
	req.Nil(metrics.byteRate)
}

// Test_lastByterates: the sim sets the byterate gauge once, when a workload finishes, so earlier events carry
// zero and the reading that counts is the last non-zero one.
func Test_lastByterates(t *testing.T) {
	req := require.New(t)
	rate := func(v float64) *ServiceMetrics { return &ServiceMetrics{byteRate: &v} }

	events := []*MetricsEvent{
		{Metrics: map[string]*ServiceMetrics{"throughput-xg": rate(0), "latency-xg": rate(0)}},
		{Metrics: map[string]*ServiceMetrics{"throughput-xg": rate(55e6), "latency-xg": rate(0)}},
		{Metrics: map[string]*ServiceMetrics{"throughput-xg": rate(0)}},
	}

	result := lastByteRates(events)
	req.Equal(55e6, result["throughput-xg"])
	_, found := result["latency-xg"]
	req.False(found, "a service whose gauge was never set reports no rate")
}
