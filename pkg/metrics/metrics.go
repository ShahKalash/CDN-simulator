package metrics

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	NodeRequests        *prometheus.CounterVec
	HTTPRequestTotal    *prometheus.CounterVec
	SegmentResponseTime *prometheus.HistogramVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		NodeRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "node_requests_total",
			Help: "Total number of requests entertained by the node, hit or miss.",
		}, []string{"type", "status"}),
		HTTPRequestTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests received by the node per endpoint.",
		}, []string{"method", "status", "route"}),
		SegmentResponseTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:                           "segment_response_time_seconds",
			Help:                           "Histogram of segment response times for different sources.",
			NativeHistogramBucketFactor:    2,
			NativeHistogramMaxBucketNumber: 25,
		}, []string{"source"}),
	}
}

func (m *Metrics) Register(registry *prometheus.Registry) error {
	if err := registry.Register(m.NodeRequests); err != nil {
		return err
	}
	if err := registry.Register(m.HTTPRequestTotal); err != nil {
		return err
	}
	if err := registry.Register(m.SegmentResponseTime); err != nil {
		return err
	}

	return nil
}
