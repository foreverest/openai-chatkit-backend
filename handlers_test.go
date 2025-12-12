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
	const expiresAfter = int64(1200)
	const rateLimit = int64(10)
	handler := newSessionHandler(fake.Create, "w", expiresAfter, rateLimit)

	req := httptest.NewRequest(http.MethodPost, "/api/chatkit/session", strings.NewReader(`{"user":"u"}`))
	rec := httptest.NewRecorder()

	handler.handleSession(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !fake.called {
		t.Fatalf("expected createSession to be called")
	}
	if fake.params.Workflow.ID != "w" {
		t.Fatalf("expected workflow_id w, got %s", fake.params.Workflow.ID)
	}
	if fake.params.ExpiresAfter.Seconds != expiresAfter {
		t.Fatalf("expected expires_after_seconds %d, got %d", expiresAfter, fake.params.ExpiresAfter.Seconds)
	}
	if !fake.params.RateLimits.MaxRequestsPer1Minute.Valid() || fake.params.RateLimits.MaxRequestsPer1Minute.Value != rateLimit {
		t.Fatalf("expected rate_limit_per_minute %d", rateLimit)
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
	handler := newSessionHandler(fake.Create, "workflow-from-env", 30, 5)

	body := `{"user":"u"}`
	req := httptest.NewRequest(http.MethodPost, "/api/chatkit/session", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.handleSession(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if fake.params.Workflow.ID != "workflow-from-env" {
		t.Fatalf("expected workflow_id workflow-from-env, got %s", fake.params.Workflow.ID)
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
		{"missing user", `{}`, http.StatusBadRequest},
		{"unknown field", `{"user":"u","workflow_id":"w","foo":1}`, http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := newSessionHandler(func(ctx context.Context, params openai.BetaChatKitSessionNewParams) (*openai.ChatSession, error) {
				t.Fatalf("createSession should not be called")
				return nil, nil
			}, "w", 1200, 10)
			req := httptest.NewRequest(http.MethodPost, "/api/chatkit/session", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			handler.handleSession(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, rec.Code)
			}
		})
	}
}
