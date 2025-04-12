package prometheus_metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	bft_metrics "github.com/hyperledger-labs/SmartBFT/pkg/metrics"
)

// PrometheusProvider реализует интерфейс Provider для Prometheus
type PrometheusProvider struct {
	namespace string
	registry  prometheus.Registerer
}

// NewPrometheusProvider создает новый экземпляр провайдера Prometheus
func NewPrometheusProvider(namespace string) *PrometheusProvider {
	return &PrometheusProvider{
		namespace: namespace,
		registry:  prometheus.DefaultRegisterer,
	}
}

// NewCounter создает счетчик Prometheus
func (p *PrometheusProvider) NewCounter(opts bft_metrics.CounterOpts) PrometheusCounter {
	counter := promauto.With(p.registry).NewCounter(prometheus.CounterOpts{
		Namespace: p.namespace,
		Name:      opts.Name,
		Help:      opts.Help,
	})
	return PrometheusCounter{counter: counter}
}

// NewGauge создает измеритель Prometheus
func (p *PrometheusProvider) NewGauge(opts bft_metrics.GaugeOpts) PrometheusGauge {
	gauge := promauto.With(p.registry).NewGauge(prometheus.GaugeOpts{
		Namespace: p.namespace,
		Name:      opts.Name,
		Help:      opts.Help,
	})
	return PrometheusGauge{gauge: gauge}
}

// NewHistogram создает гистограмму Prometheus
func (p *PrometheusProvider) NewHistogram(opts bft_metrics.HistogramOpts) PrometheusHistogram {
	histogram := promauto.With(p.registry).NewHistogram(prometheus.HistogramOpts{
		Namespace: p.namespace,
		Name:      opts.Name,
		Help:      opts.Help,
		Buckets:   opts.Buckets,
	})
	return PrometheusHistogram{histogram: histogram}
}

// PrometheusCounter обертка для счетчика Prometheus
type PrometheusCounter struct {
	counter prometheus.Counter
}

// Add увеличивает счетчик на указанное значение
func (c *PrometheusCounter) Add(delta float64) {
	c.counter.Add(delta)
}

// PrometheusGauge обертка для измерителя Prometheus
type PrometheusGauge struct {
	gauge prometheus.Gauge
}

// Set устанавливает абсолютное значение измерителя
func (g *PrometheusGauge) Set(value float64) {
	g.gauge.Set(value)
}

// Add увеличивает/уменьшает значение измерителя
func (g *PrometheusGauge) Add(delta float64) {
	g.gauge.Add(delta)
}

// PrometheusHistogram обертка для гистограммы Prometheus
type PrometheusHistogram struct {
	histogram prometheus.Histogram
}

// Observe добавляет наблюдение в гистограмму
func (h *PrometheusHistogram) Observe(value float64) {
	h.histogram.Observe(value)
}
