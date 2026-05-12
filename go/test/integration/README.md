# SOBS Health Check Integration Tests

This directory contains Go integration tests for the SOBS health check endpoints.

## Test Files

- `health_test.go` - Integration tests for `/health` and `/health/db` endpoints

## Prerequisites

1. SOBS server running (Python Quart application)
2. Go 1.25.0 or later installed
3. The server should be accessible at `http://localhost:5000` by default

## Running the Tests

### Start the SOBS server

First, ensure the SOBS application is running:

```bash
cd /home/apol/projects/sobs/sobs
python app.py
# or using hypercorn:
# hypercorn -b localhost:5000 app:app
```

### Run all integration tests

```bash
cd /home/apol/projects/sobs/sobs/go/test/integration
go test -v
```

### Run with a custom server URL

If your server is running on a different host/port:

```bash
SOBS_TEST_URL=http://localhost:8080 go test -v
```

### Run specific tests

```bash
# Run only health endpoint tests
go test -v -run TestHealthEndpoint

# Run database health tests
go test -v -run TestHealthDBEndpoint

# Run concurrent tests
go test -v -run TestHealthEndpointsConcurrent
```

### Run benchmarks

```bash
go test -bench=. -benchmem
```

## Test Cases

### TestHealthEndpoint
- Verifies `GET /health` returns HTTP 200
- Validates response is valid JSON
- Checks response time is acceptable (< 5 seconds)

### TestHealthDBEndpoint
- Verifies `GET /health/db` returns HTTP 200 or 503
- Validates response contains database status
- Checks response is valid JSON

### TestHealthEndpointsConcurrent
- Tests that the health endpoints handle 10 concurrent requests
- Verifies all concurrent requests succeed

### TestHealthEndpointWithInvalidMethod
- Tests POST to `/health` endpoint
- Verifies appropriate HTTP method handling

### TestHealthEndpointWithTestServer
- Demonstrates using `httptest` for controlled testing
- Useful for unit-style integration tests

### BenchmarkHealthEndpoint
- Provides performance benchmarking for the health endpoint
- Run with `go test -bench=BenchmarkHealthEndpoint`

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SOBS_TEST_URL` | Base URL of the SOBS server | `http://localhost:5000` |

## Notes

- Tests will skip gracefully if the server is not running
- The `waitForServer` helper waits up to 30 seconds for the server to become ready
- Use `t.Skipf()` to skip tests when the server is unavailable
- The test file includes both real server tests and `httptest` examples

## Troubleshooting

### Tests skip immediately
Ensure the SOBS server is running and accessible at the expected URL.

### Connection refused errors
Check that:
1. The server is running
2. The correct port is being used
3. No firewall is blocking the connection

### JSON parsing errors
The health endpoint response format may have changed. Check the actual response and update test assertions accordingly.
