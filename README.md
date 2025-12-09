# openai-chatkit-backend

Minimal Go HTTP server exposing a ChatKit session creation endpoint backed by the OpenAI API.

## Prerequisites
- Go 1.22+
- `OPENAI_API_KEY` set in the environment
- Optional: `OPENAI_BASE_URL` to point at a mock or custom endpoint

## Run locally
```bash
go run .
# then
curl -X POST http://localhost:8080/api/chatkit/session \
  -H "Content-Type: application/json" \
  -d '{"user":"user-123","workflow_id":"workflow-abc","expires_after_seconds":1200,"rate_limit_per_minute":10}'
```

## Build and run with Docker
```bash
docker build -t chatkit-server .
docker run --rm -p 8080:8080 -e OPENAI_API_KEY=$OPENAI_API_KEY chatkit-server
```

## Endpoint
- `POST /api/chatkit/session`
  - Request JSON: `user` (required), `workflow_id` (required), `expires_after_seconds` (optional, default 1200), `rate_limit_per_minute` (optional, default 10)
  - Response JSON: `{ "client_secret": "<secret>" }`

> Note: this server has no authentication; it’s intended for development/non-production use unless you place it behind your own auth/proxy layer.
