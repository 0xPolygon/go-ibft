package core

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics struct is used for prometheus metrics
type Metrics struct {
	roundDuration    prometheus.Histogram
	sequenceDuration prometheus.Histogram
}

// NewMetrics create new instance of metrics
func NewMetrics(roundDuration, sequenceDuration prometheus.Histogram) *Metrics {
	return &Metrics{
		roundDuration:    roundDuration,
		sequenceDuration: sequenceDuration,
	}
}
