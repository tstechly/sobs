// Package telemetry provides test instrumentation helpers for tracking test execution.
package telemetry

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// TestContext holds context for a test execution.
type TestContext struct {
	Ctx      context.Context
	span     trace.Span
	testName string
	start    time.Time
	attrs    []attribute.KeyValue
}

// StartTest begins instrumentation for a test.
func StartTest(ctx context.Context, testName string, attrs ...attribute.KeyValue) *TestContext {
	ctx, span := tracer.Start(ctx, "test.execution",
		trace.WithAttributes(
			append([]attribute.KeyValue{
				attribute.String("test.name", testName),
				attribute.String("test.suite", getTestSuiteName(testName)),
			}, attrs...)...,
		),
	)

	return &TestContext{
		Ctx:      ctx,
		span:     span,
		testName: testName,
		start:    time.Now(),
		attrs:    attrs,
	}
}

// EndTest completes test instrumentation and records metrics.
func (tc *TestContext) EndTest(passed bool, skipped bool, err error) {
	duration := time.Since(tc.start)
	durationMs := float64(duration.Milliseconds())

	// Record test duration
	if telemetryEnabled && testDuration != nil {
		testDuration.Record(tc.Ctx, durationMs,
			metric.WithAttributes(
				append([]attribute.KeyValue{
					attribute.String("test.name", tc.testName),
					attribute.String("test.suite", getTestSuiteName(tc.testName)),
					attribute.String("test.status", getTestStatus(passed, skipped)),
				}, tc.attrs...)...,
			),
		)
	}

	// Record test result counters
	if telemetryEnabled && testPassCounter != nil && passed && !skipped {
		testPassCounter.Add(tc.Ctx, 1,
			metric.WithAttributes(
				append([]attribute.KeyValue{
					attribute.String("test.name", tc.testName),
					attribute.String("test.suite", getTestSuiteName(tc.testName)),
				}, tc.attrs...)...,
			),
		)
	}

	if telemetryEnabled && testFailCounter != nil && !passed && !skipped {
		testFailCounter.Add(tc.Ctx, 1,
			metric.WithAttributes(
				append([]attribute.KeyValue{
					attribute.String("test.name", tc.testName),
					attribute.String("test.suite", getTestSuiteName(tc.testName)),
				}, tc.attrs...)...,
			),
		)

		// Record error if provided
		if telemetryEnabled && errorCounter != nil {
			errorCounter.Add(tc.Ctx, 1,
				metric.WithAttributes(
					append([]attribute.KeyValue{
						attribute.String("error.type", "test_failure"),
						attribute.String("test.name", tc.testName),
					}, tc.attrs...)...,
				),
			)
		}
	}

	if telemetryEnabled && testSkipCounter != nil && skipped {
		testSkipCounter.Add(tc.Ctx, 1,
			metric.WithAttributes(
				append([]attribute.KeyValue{
					attribute.String("test.name", tc.testName),
					attribute.String("test.suite", getTestSuiteName(tc.testName)),
				}, tc.attrs...)...,
			),
		)
	}

	// Set span status
	if skipped {
		tc.span.SetStatus(codes.Ok, "Test skipped")
	} else if passed {
		tc.span.SetStatus(codes.Ok, "Test passed")
	} else {
		tc.span.SetStatus(codes.Error, "Test failed")
	}

	// Record error in span if provided
	if err != nil {
		tc.span.RecordError(err)
		tc.span.AddEvent("error")
	}

	// End span
	tc.span.End()
}

// RecordDBQuery records a database query with metrics.
func RecordDBQuery(ctx context.Context, queryType string, table string, duration time.Duration, err error) {
	durationMs := float64(duration.Milliseconds())

	// Record query counter
	if telemetryEnabled && dbQueryCounter != nil {
		dbQueryCounter.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("db.query.type", queryType),
				attribute.String("db.table", table),
			),
		)
	}

	// Record query duration
	if telemetryEnabled && dbQueryDuration != nil {
		dbQueryDuration.Record(ctx, durationMs,
			metric.WithAttributes(
				attribute.String("db.query.type", queryType),
				attribute.String("db.table", table),
			),
		)
	}

	// Record error if query failed
	if err != nil && telemetryEnabled && errorCounter != nil {
		errorCounter.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("error.type", "db_query_error"),
				attribute.String("db.query.type", queryType),
				attribute.String("db.table", table),
			),
		)
	}
}

// RecordError records an error with context.
func RecordError(ctx context.Context, errorType string, err error, attrs ...attribute.KeyValue) {
	if telemetryEnabled && errorCounter != nil {
		allAttrs := append([]attribute.KeyValue{
			attribute.String("error.type", errorType),
		}, attrs...)
		errorCounter.Add(ctx, 1,
			metric.WithAttributes(allAttrs...),
		)
	}
}

// getTestSuiteName extracts the suite name from the test name.
// Test names are typically in the format "TestSuite/TestName" or just "TestName".
func getTestSuiteName(testName string) string {
	parts := strings.Split(testName, "/")
	if len(parts) > 1 {
		return parts[0]
	}
	return "default"
}

// getTestStatus returns the test status string.
func getTestStatus(passed, skipped bool) string {
	switch {
	case skipped:
		return "skipped"
	case passed:
		return "passed"
	default:
		return "failed"
	}
}

// RunTestWithTelemetry wraps a test function with telemetry instrumentation.
func RunTestWithTelemetry(t *testing.T, testName string, testFunc func(ctx context.Context) error) {
	tc := StartTest(context.Background(), testName,
		attribute.String("test.runner", "go-test"),
	)

	defer func() {
		// Recover from panic
		if r := recover(); r != nil {
			t.Errorf("Test panicked: %v", r)
			var testErr error
			if e, ok := r.(error); ok {
				testErr = e
			} else {
				testErr = fmt.Errorf("panic: %v", r)
			}
			tc.EndTest(false, false, testErr)
		}
	}()

	// Run test function
	testErr := testFunc(tc.Ctx)

	// Determine test result
	passed := testErr == nil
	skipped := false

	// Check if test was skipped
	if t.Skipped() {
		skipped = true
		passed = true
	}

	tc.EndTest(passed, skipped, testErr)

	if testErr != nil {
		t.Errorf("Test failed: %v", testErr)
	}
}