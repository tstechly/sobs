# SOBS Integration Tests OpenTelemetry Instrumentation

This package provides OpenTelemetry instrumentation for SOBS integration tests, enabling the test suite to monitor itself with traces and metrics sent to an OTLP endpoint.

## Features

- **HTTP Request Instrumentation**: Automatic tracing and metrics for all HTTP requests made during tests
- **Test Execution Metrics**: Records test duration, pass/fail/skip counts
- **Database Query Tracking**: Optional instrumentation for database operations
- **Error Tracking**: Captures and records errors with context
- **Context Propagation**: Ensures trace context flows through async operations

## Architecture

```
go/telemetry/
├── telemetry.go       # Core initialization (TracerProvider, MeterProvider)
├── httpclient.go      # Instrumented HTTP client
├── testhelper.go      # Test execution instrumentation helpers
└── README.md          # This file
```

## Usage

### Initialization

Telemetry is automatically initialized in `TestMain`:
```go
func TestMain(m *testing.M) {
    // Telemetry initialized automatically
    exitCode := m.Run()
    os.Exit(exitCode)
}
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SOBS_OTEL_ENDPOINT` | OTLP endpoint for sending telemetry | `localhost:44317` |
| `SOBS_OTEL_DISABLED` | Disable telemetry (set to `"true"`) | not set |
| `OTEL_SDK_DISABLED` | Disable OpenTelemetry SDK | not set |
| `DEPLOYMENT_ENVIRONMENT` | Deployment environment attribute | `development` |

### In Tests

```go
func TestMyEndpoint(t *testing.T) {
    // Initialize test context with telemetry
    tc, cleanup := InitTest(t, "TestMyEndpoint")
    defer cleanup()

    // Use the instrumented HTTP client
    ctx := context.Background()
    if tc != nil {
        ctx = tc.ctx
    }

    resp, err := sobstelemetry.Get(ctx, baseURL + "/api/endpoint")
    if err != nil {
        t.Fatalf("Request failed: %v", err)
    }
    defer resp.Body.Close()

    // Test assertions...
}
```

## Metrics Collected

### HTTP Client Metrics
- `http.client.requests` - Counter of HTTP requests
- `http.client.request.duration` - Histogram of request durations (ms)
- `http.client.request.body.size` - Histogram of request body sizes (bytes)
- `http.client.response.body.size` - Histogram of response body sizes (bytes)

### Test Execution Metrics
- `test.execution.duration` - Histogram of test durations (ms)
- `test.results.pass` - Counter of passed tests
- `test.results.fail` - Counter of failed tests
- `test.results.skip` - Counter of skipped tests

### Database Metrics (Optional)
- `db.queries` - Counter of database queries
- `db.queries.duration` - Histogram of query durations (ms)

### Error Metrics
- `errors.total` - Counter of errors with type attribution

## Traces Collected

Each HTTP request and test execution creates a span with attributes:
- `http.method`, `http.url`, `http.host`, `http.status_code`
- `test.name`, `test.suite`, `test.status`
- `error.type`, `http.status_category`

## Service Resource Attributes

The instrumentation automatically adds service metadata:
```json
{
  "service.name": "sobs-integration-tests",
  "service.version": "1.0.0",
  "telemetry.sdk.name": "opentelemetry",
  "telemetry.sdk.language": "go",
  "telemetry.sdk.version": "1.31.0",
  "deployment.environment": "development",
  "app.name": "sobs-integration-tests",
  "app.component": "integration-test-suite"
}
```

## Running Tests with Telemetry

### Send to SOBS (Default)
```bash
# Run tests - telemetry sent to localhost:44317
cd go
go test ./test/integration/...
```

### Send to Custom OTLP Endpoint
```bash
export SOBS_OTEL_ENDPOINT=my-collector:4317
go test ./test/integration/...
```

### Disable Telemetry
```bash
export SOBS_OTEL_DISABLED=true
go test ./test/integration/...
```

## Implementation Details

### HTTP Instrumentation
The package uses `otelhttp` transport for automatic HTTP client instrumentation. Each request:
1. Creates a span with HTTP attributes
2. Records request/response timing
3. Emits metrics for duration, body sizes, and status codes
4. Propagates trace context via headers

### Test Instrumentation
Test wrapping:
1. Creates a span at test start
2. Records test duration on completion
3. Emits pass/fail/skip metrics
4. Captures errors with stack traces

### Error Handling
When telemetry is disabled or fails:
- All recording operations become no-ops
- Tests continue to run normally
- No performance impact from disabled telemetry

## View Telemetry in SOBS

After running tests, navigate to:
- **Traces**: Browse to AI → Traces, filter by `service.name: sobs-integration-tests`
- **Metrics**: Browse to Metrics → Create Dashboard, select HTTP and test metrics
- **Dashboard**: Query `otel_metrics_1m` table for aggregated data

Example queries:
```sql
-- HTTP request latency percentiles
SELECT
    quantile(0.50)(value) as p50,
    quantile(0.95)(value) as p95,
    quantile(0.99)(value) as p99
FROM otel_metrics_1m
WHERE metric_name = 'http.client.request.duration'
AND service_name = 'sobs-integration-tests'

-- Test pass rate over time
SELECT
    time,
    sumIf(value, attribute('test.status', 'passed')) as passed,
    sumIf(value, attribute('test.status', 'failed')) as failed
FROM otel_metrics_1m
WHERE metric_name = 'test.results.pass'
GROUP BY time
ORDER BY time
```

## Troubleshooting

### Telemetry not appearing
1. Check SOBS is running: `curl http://localhost:44317/health`
2. Verify endpoint: `echo $SOBS_OTEL_ENDPOINT`
3. Check logs for initialization errors
4. Try disabling firewall blocking port 4317

### Tests failing with telemetry enabled
1. Disable telemetry temporarily: `export SOBS_OTEL_DISABLED=true`
2. Run tests to diagnose unrelated issues
3. Check network connectivity to OTLP endpoint

### Performance issues
1. Metrics are batched and exported every 10 seconds
2. Spans are batched and exported on test suite completion
3. Minimal overhead when using default settings

## Dependencies

```go
go.opentelemetry.io/otel v1.31.0
go.opentelemetry.io/otel/sdk v1.31.0
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.31.0
go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.31.0
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.57.0
```

## License

Same as SOBS project license.