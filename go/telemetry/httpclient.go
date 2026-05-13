// Package telemetry provides instrumented HTTP client for monitoring HTTP requests.
package telemetry

import (
	"context"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

var (
	// Default HTTP client with OpenTelemetry instrumentation
	defaultClient *http.Client
)

// InitHTTPClient initializes the default instrumented HTTP client.
func InitHTTPClient() *http.Client {
	if defaultClient != nil {
		return defaultClient
	}

	// Create transport with OpenTelemetry instrumentation
	transport := otelhttp.NewTransport(http.DefaultTransport,
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)

	// Create instrumented HTTP client
	defaultClient = &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return defaultClient
}

// GetClient returns the instrumented HTTP client.
func GetClient() *http.Client {
	if defaultClient == nil {
		return InitHTTPClient()
	}
	return defaultClient
}

// Do performs an HTTP request with manual metric recording for additional telemetry.
func Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	client := GetClient()

	// Start span for additional context
	_, span := tracer.Start(ctx, "http.client.request",
		trace.WithAttributes(
			attribute.String("http.method", req.Method),
			attribute.String("http.url", req.URL.String()),
			attribute.String("http.host", req.Host),
			attribute.String("http.scheme", req.URL.Scheme),
		),
	)
	defer span.End()

	startTime := time.Now()

	// Record request body size if available
	if req.Body != nil {
		// Try to get content length header first
		if contentLength := req.ContentLength; contentLength > 0 {
			if telemetryEnabled && requestBodySizeBytes != nil {
				requestBodySizeBytes.Record(ctx, contentLength,
					metric.WithAttributes(
						attribute.String("http.method", req.Method),
						attribute.String("http.host", req.Host),
					),
				)
			}
		}
	}

	// Execute request
	resp, err := client.Do(req)

	duration := time.Since(startTime)
	durationMs := float64(duration.Milliseconds())

	// Record request duration
	if telemetryEnabled && httpRequestDuration != nil {
		httpRequestDuration.Record(ctx, durationMs,
			metric.WithAttributes(
				attribute.String("http.method", req.Method),
				attribute.String("http.host", req.Host),
			),
		)
	}

	if err != nil {
		// Record error
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		if telemetryEnabled && errorCounter != nil {
			errorCounter.Add(ctx, 1,
				metric.WithAttributes(
					attribute.String("error.type", "http_client_error"),
					attribute.String("http.method", req.Method),
					attribute.String("http.host", req.Host),
				),
			)
		}

		return nil, err
	}

	// Record request counter
	if telemetryEnabled && httpRequestCounter != nil {
		httpRequestCounter.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("http.method", req.Method),
				attribute.String("http.url", req.URL.String()),
				attribute.String("http.host", req.Host),
				attribute.Int("http.status_code", resp.StatusCode),
				attribute.String("http.status_category", getStatusCategory(resp.StatusCode)),
			),
		)
	}

	// Record response body size
	if resp.ContentLength > 0 {
		if telemetryEnabled && responseBodySizeBytes != nil {
			responseBodySizeBytes.Record(ctx, resp.ContentLength,
				metric.WithAttributes(
					attribute.String("http.method", req.Method),
					attribute.String("http.host", req.Host),
					attribute.Int("http.status_code", resp.StatusCode),
				),
			)
		}
	}

	// Set span attributes from response
	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
		attribute.String("http.status_text", resp.Status),
		attribute.String("http.status_category", getStatusCategory(resp.StatusCode)),
	)

	// Set span status based on status code
	if resp.StatusCode >= 400 {
		span.SetStatus(codes.Error, "HTTP request failed")
	} else {
		span.SetStatus(codes.Ok, "HTTP request succeeded")
	}

	return resp, nil
}

// Get performs an HTTP GET request with instrumentation.
func Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return Do(ctx, req)
}

// Post performs an HTTP POST request with instrumentation.
func Post(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return Do(ctx, req)
}

// Put performs an HTTP PUT request with instrumentation.
func Put(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return Do(ctx, req)
}

// Delete performs an HTTP DELETE request with instrumentation.
func Delete(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}
	return Do(ctx, req)
}

// getStatusCategory returns the HTTP status category based on the status code.
func getStatusCategory(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return "2xx"
	case statusCode >= 300 && statusCode < 400:
		return "3xx"
	case statusCode >= 400 && statusCode < 500:
		return "4xx"
	case statusCode >= 500 && statusCode < 600:
		return "5xx"
	default:
		return "unknown"
	}
}

// RequestWrapper wraps an http.HandlerFunc with OpenTelemetry instrumentation.
func RequestWrapper(operation string, handler http.Handler) http.Handler {
	return otelhttp.NewHandler(handler, operation,
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return operation + " " + r.Method + " " + r.URL.Path
		}),
	)
}

// Middleware wraps an HTTP handler with OpenTelemetry instrumentation for use as middleware.
func Middleware(operation string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, operation)
	}
}