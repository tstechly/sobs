# Authentication Setup

SOBS supports two independent auth areas:

- Ingest API auth for `/v1/*` via `SOBS_API_KEY`
- Web UI auth for `/`, `/logs`, `/errors`, `/traces`, `/rum`, `/ai`

Web UI auth mode is exclusive. Configure exactly one mode:

- none
- basic
- external

If the configuration is mixed or incomplete, SOBS returns `500` with:

`{"error": "Server auth misconfiguration"}`

## 1) Ingest API auth (`SOBS_API_KEY`)

Set `SOBS_API_KEY` to require an API key for ingest endpoints.

Example environment:

    SOBS_API_KEY=my-ingest-key

Send data with header:

    X-API-Key: my-ingest-key

Example request:

    curl -X POST http://localhost:4317/v1/logs \
      -H "Content-Type: application/json" \
      -H "X-API-Key: my-ingest-key" \
      -d '{}'

## 2) Web UI mode: none (no UI auth)

Leave all UI auth variables empty:

    SOBS_BASIC_AUTH_USERNAME=
    SOBS_BASIC_AUTH_PASSWORD=
    SOBS_EXTERNAL_AUTH_URL=

Result:

- Web UI routes are open

## 3) Web UI mode: basic

Set both Basic credentials, and do not set external auth:

    SOBS_BASIC_AUTH_USERNAME=admin
    SOBS_BASIC_AUTH_PASSWORD=secret
    SOBS_EXTERNAL_AUTH_URL=

Result:

- Web UI requires `Authorization: Basic ...`
- Unauthorized responses include `WWW-Authenticate: Basic realm="SOBS"`

Example (generates Basic header using base64):

    curl -i http://localhost:4317/ \
      -H "Authorization: Basic $(printf 'admin:secret' | base64)"

## 4) Web UI mode: external

Set external validator URL only:

    SOBS_EXTERNAL_AUTH_URL=http://auth-service
    SOBS_BASIC_AUTH_USERNAME=
    SOBS_BASIC_AUTH_PASSWORD=

SOBS validates bearer tokens by POSTing to:

`{SOBS_EXTERNAL_AUTH_URL}/internal/auth/validate`

It forwards the incoming `Authorization` header and accepts the request only when validation returns HTTP `200`.

Result:

- Web UI requires `Authorization: Bearer ...`
- Unauthorized responses include `WWW-Authenticate: Bearer realm="SOBS"`

Example request:

    curl -i http://localhost:4317/ \
      -H "Authorization: Bearer eyJhbGciOi..."

## 5) Invalid UI auth configurations

These are invalid and will return `500` on Web UI routes:

- Only one of `SOBS_BASIC_AUTH_USERNAME` or `SOBS_BASIC_AUTH_PASSWORD` is set
- Basic credentials are set and `SOBS_EXTERNAL_AUTH_URL` is also set

## Docker Compose example

Basic mode:

    services:
      sobs:
        image: sobs:latest
        ports:
          - "4317:4317"
        environment:
          - SOBS_API_KEY=my-ingest-key
          - SOBS_BASIC_AUTH_USERNAME=admin
          - SOBS_BASIC_AUTH_PASSWORD=secret

External mode:

    services:
      sobs:
        image: sobs:latest
        ports:
          - "4317:4317"
        environment:
          - SOBS_API_KEY=my-ingest-key
          - SOBS_EXTERNAL_AUTH_URL=http://auth-service

## Quick copy/paste setups

### Local dev (.env style)

No UI auth:

    SOBS_API_KEY=my-ingest-key
    SOBS_BASIC_AUTH_USERNAME=
    SOBS_BASIC_AUTH_PASSWORD=
    SOBS_EXTERNAL_AUTH_URL=

Basic UI auth:

    SOBS_API_KEY=my-ingest-key
    SOBS_BASIC_AUTH_USERNAME=admin
    SOBS_BASIC_AUTH_PASSWORD=secret
    SOBS_EXTERNAL_AUTH_URL=

External UI auth:

    SOBS_API_KEY=my-ingest-key
    SOBS_BASIC_AUTH_USERNAME=
    SOBS_BASIC_AUTH_PASSWORD=
    SOBS_EXTERNAL_AUTH_URL=http://auth-service

### Docker Compose environment block

Basic UI auth:

    environment:
      - SOBS_API_KEY=my-ingest-key
      - SOBS_BASIC_AUTH_USERNAME=admin
      - SOBS_BASIC_AUTH_PASSWORD=secret

External UI auth:

    environment:
      - SOBS_API_KEY=my-ingest-key
      - SOBS_EXTERNAL_AUTH_URL=http://auth-service

### Kubernetes quick commands

Basic UI auth:

    kubectl create secret generic sobs-auth \
      --from-literal=SOBS_API_KEY=my-ingest-key \
      --from-literal=SOBS_BASIC_AUTH_USERNAME=admin \
      --from-literal=SOBS_BASIC_AUTH_PASSWORD=secret

External UI auth:

    kubectl create secret generic sobs-auth \
      --from-literal=SOBS_API_KEY=my-ingest-key \
      --from-literal=SOBS_EXTERNAL_AUTH_URL=http://auth-service

Then reference the secret in your Deployment environment (`envFrom.secretRef` or per-key `env.valueFrom.secretKeyRef`).
