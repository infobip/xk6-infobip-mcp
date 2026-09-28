package infobip_mcp

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"go.k6.io/k6/lib"
	"go.k6.io/k6/metrics"
)

type RoundTripper struct {
	headers   map[string]string
	transport http.RoundTripper
	metrics   *MCPMetrics
	state     *lib.State
}

// RoundTrip implements the http.RoundTripper interface. It intercepts HTTP requests
// to add custom headers and collect detailed metrics
func (r RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add custom headers
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := r.transport.RoundTrip(req)
	duration := time.Since(start)

	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}

	tags := r.state.Tags.GetCurrentValues().Tags.WithTagsFromMap(map[string]string{
		"method": req.Method,
		"url":    req.URL.String(),
		"status": strconv.Itoa(statusCode),
	})
	now := time.Now()

	failureValue := 1.0
	if err == nil {
		switch {
		case statusCode < 400:
			failureValue = 0.0
		case statusCode == http.StatusNotFound && req.Method == http.MethodDelete:
			// Session already gone on the server (expired or stateless); not a failure.
			failureValue = 0.0
		case statusCode == http.StatusMethodNotAllowed && req.Method == http.MethodDelete:
			// Stateless servers do not implement DELETE; not a failure.
			failureValue = 0.0
		case statusCode == http.StatusMethodNotAllowed && req.Method == http.MethodGet:
			// Server offers no standalone SSE stream (spec-allowed, typical for
			// stateless servers); not a failure.
			failureValue = 0.0
		}
	}

	metrics.PushIfNotDone(context.Background(), r.state.Samples, metrics.ConnectedSamples{
		Samples: []metrics.Sample{
			{
				TimeSeries: metrics.TimeSeries{Metric: r.metrics.HTTPRequestDuration, Tags: tags},
				Time:       now,
				Value:      metrics.D(duration),
			},
			{
				TimeSeries: metrics.TimeSeries{Metric: r.metrics.HTTPRequestCount, Tags: tags},
				Time:       now,
				Value:      1,
			},
			{
				TimeSeries: metrics.TimeSeries{Metric: r.metrics.HTTPRequestErrors, Tags: tags},
				Time:       now,
				Value:      failureValue,
			},
		},
		Tags: tags,
		Time: now,
	})

	return resp, err
}
