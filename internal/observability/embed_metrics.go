package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	embedCacheLookups = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agent_memory_embed_cache_lookups_total",
		Help: "Total embedding cache lookups grouped by provider, model, and result.",
	}, []string{"provider", "model", "result"})

	embedProviderRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agent_memory_embed_provider_requests_total",
		Help: "Total embedding provider requests grouped by provider, model, and result.",
	}, []string{"provider", "model", "result"})

	embedProviderInputTexts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agent_memory_embed_provider_input_texts_total",
		Help: "Total input texts sent to the embedding provider.",
	}, []string{"provider", "model"})

	embedProviderDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agent_memory_embed_provider_request_duration_seconds",
		Help:    "Latency of embedding provider requests.",
		Buckets: prometheus.DefBuckets,
	}, []string{"provider", "model"})
)

// RecordEmbedCacheLookup records an embedding cache lookup outcome.
func RecordEmbedCacheLookup(provider, model, result string) {
	embedCacheLookups.WithLabelValues(provider, model, result).Inc()
}

// RecordEmbedProviderRequest records a provider request outcome, duration, and input count.
func RecordEmbedProviderRequest(provider, model string, inputTexts int, duration time.Duration, err error) {
	result := "success"
	if err != nil {
		result = "error"
	}
	embedProviderRequests.WithLabelValues(provider, model, result).Inc()
	embedProviderDuration.WithLabelValues(provider, model).Observe(duration.Seconds())
	embedProviderInputTexts.WithLabelValues(provider, model).Add(float64(inputTexts))
}
