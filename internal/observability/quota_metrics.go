package observability

import "github.com/prometheus/client_golang/prometheus"
import "github.com/prometheus/client_golang/prometheus/promauto"

var httpRateLimitDenied = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "agent_memory_http_rate_limit_denied_total",
	Help: "Total HTTP requests denied by the in-memory rate limiter.",
}, []string{"path"})

// RecordHTTPRateLimitDenied records a throttled request for one path.
func RecordHTTPRateLimitDenied(path string) {
	httpRateLimitDenied.WithLabelValues(path).Inc()
}
