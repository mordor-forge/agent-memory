package apicontract

const (
	// HTTPAPIVersion is the major HTTP API line exposed under /v1/.
	HTTPAPIVersion = "v1"
	// HTTPMediaType is the canonical JSON media type for versioned HTTP responses.
	HTTPMediaType = "application/vnd.agent-memory.v1+json"
	// CompatibilityStability communicates the current maturity of the public surfaces.
	CompatibilityStability = "alpha"
	// HTTPVersionHeader advertises the active HTTP API version on /v1/ responses.
	HTTPVersionHeader = "X-Agent-Memory-API-Version"
	// HTTPStabilityHeader advertises the compatibility stability of the current API line.
	HTTPStabilityHeader = "X-Agent-Memory-API-Stability"
)
