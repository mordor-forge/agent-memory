package observability

import (
	"net/http"

	"github.com/mordor-forge/agent-memory/internal/version"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var buildInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "agent_memory_build_info",
	Help: "Build information for the running agent-memory binary.",
}, []string{"name", "version", "commit", "date"})

// RegisterBuildInfo exposes the current binary version through Prometheus.
func RegisterBuildInfo(info version.Info) {
	buildInfo.WithLabelValues(info.Name, info.Version, info.Commit, info.Date).Set(1)
}

// MetricsHandler returns the Prometheus scrape endpoint.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
