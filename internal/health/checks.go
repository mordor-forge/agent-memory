package health

import (
	"context"
	"errors"
)

// Checker is a readiness probe dependency.
type Checker interface {
	Name() string
	Check(context.Context) error
}

// Result captures one health check outcome.
type Result struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Report is a readiness summary across all checks.
type Report struct {
	Status  string   `json:"status"`
	Results []Result `json:"results"`
}

// Evaluate runs the provided checks and returns a report.
func Evaluate(ctx context.Context, checkers []Checker) Report {
	report := Report{Status: "ok", Results: make([]Result, 0, len(checkers))}
	for _, checker := range checkers {
		err := checker.Check(ctx)
		result := Result{Name: checker.Name(), OK: err == nil}
		if err != nil {
			report.Status = "degraded"
			result.Error = err.Error()
		}
		report.Results = append(report.Results, result)
	}
	return report
}

// PingChecker adapts a ping-like function into a readiness check.
type PingChecker struct {
	NameValue string
	PingFunc  func(context.Context) error
}

// Name returns the check name.
func (c PingChecker) Name() string {
	return c.NameValue
}

// Check executes the wrapped ping function.
func (c PingChecker) Check(ctx context.Context) error {
	if c.PingFunc == nil {
		return errors.New("nil ping function")
	}
	return c.PingFunc(ctx)
}
