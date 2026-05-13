// Package integration provides test initialization and setup for SOBS integration tests.
package integration

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"

	sobstelemetry "github.com/abartrim/sobs/go/telemetry"
)

const (
	// Service metadata for telemetry
	serviceName    = "sobs-integration-tests"
	serviceVersion = "1.0.0"
)

var (
	telemetryInitialized bool
	shutdownFunc         func(context.Context) error
)

// TestMain is the entry point for all integration tests.
// It initializes OpenTelemetry and performs cleanup after tests complete.
func TestMain(m *testing.M) {
	log.Println("[integration] Starting SOBS integration tests")

	// Initialize OpenTelemetry
	if err := initTelemetry(); err != nil {
		log.Printf("[integration] Warning: failed to initialize telemetry: %v", err)
	} else {
		telemetryInitialized = true
		// Ensure telemetry shutdown runs on exit
		defer shutdownTelemetry()
	}

	// Run tests
	exitCode := m.Run()

	log.Printf("[integration] Tests completed with exit code: %d", exitCode)
	os.Exit(exitCode)
}

// initTelemetry initializes OpenTelemetry instrumentation for the test suite.
func initTelemetry() error {
	// Check if telemetry is disabled via environment variable
	if os.Getenv("SOBS_OTEL_DISABLED") == "true" || os.Getenv("OTEL_SDK_DISABLED") == "true" {
		log.Println("[integration] OpenTelemetry disabled via environment variable")
		return nil
	}

	// Get OTLP endpoint from environment or use default
	otlpEndpoint := os.Getenv("SOBS_OTEL_ENDPOINT")
	if otlpEndpoint == "" {
		otlpEndpoint = sobstelemetry.DefaultOTLPEndpoint
	}

	// Initialize telemetry
	if err := sobstelemetry.Init(serviceName, serviceVersion,
		sobstelemetry.WithOTLPEndpoint(otlpEndpoint),
	); err != nil {
		return fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	// Initialize HTTP client
	sobstelemetry.InitHTTPClient()

	shutdownFunc = sobstelemetry.Shutdown

	log.Printf("[integration] OpenTelemetry initialized (endpoint: %s)", otlpEndpoint)

	return nil
}

// shutdownTelemetry gracefully shuts down OpenTelemetry instrumentation.
func shutdownTelemetry() {
	if !telemetryInitialized || shutdownFunc == nil {
		return
	}

	log.Println("[integration] Shutting down OpenTelemetry...")
	ctx, cancel := context.WithTimeout(context.Background(), 10)
	defer cancel()

	if err := shutdownFunc(ctx); err != nil {
		log.Printf("[integration] Warning: telemetry shutdown error: %v", err)
	}

	log.Println("[integration] OpenTelemetry shutdown complete")
}

// InitTest initializes a single test with telemetry context.
// Returns a test context and a cleanup function.
func InitTest(t *testing.T, testName string) (*sobstelemetry.TestContext, func()) {
	t.Cleanup(func() {
		// Test cleanup code
	})

	// If telemetry is not initialized, return a no-op context
	if !telemetryInitialized {
		return nil, func() {}
	}

	// Create test context with telemetry
	tc := sobstelemetry.StartTest(context.Background(), testName)

	// Return cleanup function that ends the test
	return tc, func() {
		passed := !t.Failed()
		skipped := t.Skipped()
		var testErr error
		if !passed && !skipped {
			testErr = fmt.Errorf("test %s failed", testName)
		}
		tc.EndTest(passed, skipped, testErr)
	}
}

// RecordHTTPError records an HTTP error with telemetry.
func RecordHTTPError(testName string, statusCode int, err error) {
	if !telemetryInitialized {
		return
	}

	ctx := context.Background()
	sobstelemetry.RecordError(ctx, "http_error", err)
}

// withAttributes adds attributes to the context (helper function).
// Note: This is a simplified version - in production you'd use proper attribute handling.
func withAttributes(ctx context.Context, kvPairs ...string) context.Context {
	// This is a placeholder - in production you'd use proper attribute propagation
	// For now, we just return the context as-is
	// The actual attributes would be passed via the RecordError function calls
	return ctx
}

// GetInstrumentedClient returns the instrumented HTTP client for tests.
func GetInstrumentedClient() *http.Client {
	return sobstelemetry.GetClient()
}