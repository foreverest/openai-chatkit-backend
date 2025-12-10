# openai-chatkit-backend

Minimal Go HTTP server exposing a ChatKit session creation endpoint backed by the OpenAI API.

## Prerequisites
- Go 1.22+
- Required environment variables:
  - `OPENAI_API_KEY`: API key used to call the OpenAI API.
  - `CHATKIT_WORKFLOW_ID`: ChatKit workflow ID the server will use for every session.
  - `CHATKIT_EXPIRES_AFTER_SECONDS`: Lifetime (seconds) to set on each created session.
  - `CHATKIT_RATE_LIMIT_PER_MINUTE`: Per-minute request limit to set on each created session.
- Optional: `OPENAI_BASE_URL` to point at a mock or custom endpoint
> These values must be provided explicitly; there are no defaults for expiry or rate limits.

## Run locally
```bash
export OPENAI_API_KEY=sk-...
export CHATKIT_WORKFLOW_ID=workflow-abc
export CHATKIT_EXPIRES_AFTER_SECONDS=1200
export CHATKIT_RATE_LIMIT_PER_MINUTE=10
go run .
# then
curl -X POST http://localhost:8080/api/chatkit/session \
  -H "Content-Type: application/json" \
  -d '{"user":"user-123"}'
```

## Build and run with Docker
```bash
docker build -t chatkit-server .
docker run --rm -p 8080:8080 \
  -e OPENAI_API_KEY=$OPENAI_API_KEY \
  -e CHATKIT_WORKFLOW_ID=$CHATKIT_WORKFLOW_ID \
  -e CHATKIT_EXPIRES_AFTER_SECONDS=$CHATKIT_EXPIRES_AFTER_SECONDS \
  -e CHATKIT_RATE_LIMIT_PER_MINUTE=$CHATKIT_RATE_LIMIT_PER_MINUTE \
  chatkit-server
```
The provided multi-stage Dockerfile produces a tiny (~10MB) scratch-based image.

## Endpoint
- `POST /api/chatkit/session`
  - Request JSON: `user` (required)
  - Response JSON: `{ "client_secret": "<secret>" }`

> Note: this server has no authentication; it’s intended for development/non-production use unless you place it behind your own auth/proxy layer.
