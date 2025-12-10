package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openai "github.com/openai/openai-go/v3"
)

type fakeSessionCreator struct {
	clientSecret string
	err          error

	called bool
	params openai.BetaChatKitSessionNewParams
}

func (f *fakeSessionCreator) Create(ctx context.Context, params openai.BetaChatKitSessionNewParams) (*openai.ChatSession, error) {
	f.called = true
	f.params = params
	if f.err != nil {
		return nil, f.err
	}
	return &openai.ChatSession{ClientSecret: f.clientSecret}, nil
}

func TestHandleSessionDefaults(t *testing.T) {
	fake := &fakeSessionCreator{clientSecret: "secret"}
	srv := &server{createSession: fake.Create}

	req := httptest.NewRequest(http.MethodPost, "/api/chatkit/session", strings.NewReader(`{"user":"u","workflow_id":"w"}`))
	rec := httptest.NewRecorder()

	srv.handleSession(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !fake.called {
		t.Fatalf("expected createSession to be called")
	}
	if fake.params.ExpiresAfter.Seconds != defaultExpiresAfterSecs {
		t.Fatalf("expected default expires_after_seconds %d, got %d", defaultExpiresAfterSecs, fake.params.ExpiresAfter.Seconds)
	}
	if !fake.params.RateLimits.MaxRequestsPer1Minute.Valid() || fake.params.RateLimits.MaxRequestsPer1Minute.Value != defaultRateLimitPerMin {
		t.Fatalf("expected default rate_limit_per_minute %d", defaultRateLimitPerMin)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["client_secret"] != "secret" {
		t.Fatalf("unexpected client_secret: %s", resp["client_secret"])
	}
}

func TestHandleSessionWithValues(t *testing.T) {
	fake := &fakeSessionCreator{clientSecret: "secret2"}
	srv := &server{createSession: fake.Create}

	body := `{"user":"u","workflow_id":"w","expires_after_seconds":30,"rate_limit_per_minute":5}`
	req := httptest.NewRequest(http.MethodPost, "/api/chatkit/session", strings.NewReader(body))
	rec := httptest.NewRecorder()

	srv.handleSession(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if fake.params.ExpiresAfter.Seconds != 30 {
		t.Fatalf("expected expires_after_seconds 30, got %d", fake.params.ExpiresAfter.Seconds)
	}
	if !fake.params.RateLimits.MaxRequestsPer1Minute.Valid() || fake.params.RateLimits.MaxRequestsPer1Minute.Value != 5 {
		t.Fatalf("expected rate_limit_per_minute 5")
	}
}

func TestHandleSessionValidationErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"missing user", `{"workflow_id":"w"}`, http.StatusBadRequest},
		{"missing workflow", `{"user":"u"}`, http.StatusBadRequest},
		{"negative expiry", `{"user":"u","workflow_id":"w","expires_after_seconds":-1}`, http.StatusBadRequest},
		{"negative rate", `{"user":"u","workflow_id":"w","rate_limit_per_minute":-1}`, http.StatusBadRequest},
		{"unknown field", `{"user":"u","workflow_id":"w","foo":1}`, http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := &server{createSession: func(ctx context.Context, params openai.BetaChatKitSessionNewParams) (*openai.ChatSession, error) {
				t.Fatalf("createSession should not be called")
				return nil, nil
			}}
			req := httptest.NewRequest(http.MethodPost, "/api/chatkit/session", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			srv.handleSession(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, rec.Code)
			}
		})
	}
}
