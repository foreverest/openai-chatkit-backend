package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddlewareAllowsOrigin(t *testing.T) {
	policy := newCORSPolicy("https://app.example.com")
	handler := withCORS(policy, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/chatkit/session", nil)
	req.Header.Set("Origin", "https://app.example.com")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}
	if res.Header.Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf("missing allowed origin header")
	}
	if !containsHeader(res.Header["Vary"], "Origin") {
		t.Fatalf("expected Vary: Origin header")
	}
}

func TestCORSMiddlewareRejectsOrigin(t *testing.T) {
	policy := newCORSPolicy("https://app.example.com")
	handler := withCORS(policy, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for rejected origin")
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/chatkit/session", nil)
	req.Header.Set("Origin", "https://evil.example.com")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestCORSMiddlewarePreflight(t *testing.T) {
	policy := newCORSPolicy("*")
	called := false
	handler := withCORS(policy, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/chatkit/session", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatalf("handler should not be invoked for preflight")
	}
	res := rec.Result()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", res.StatusCode)
	}
	if res.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("expected wildcard allowed origin")
	}
	if res.Header.Get("Access-Control-Allow-Methods") != "GET, POST, OPTIONS" {
		t.Fatalf("unexpected allowed methods: %s", res.Header.Get("Access-Control-Allow-Methods"))
	}
	if res.Header.Get("Access-Control-Allow-Headers") == "" {
		t.Fatalf("expected allowed headers to be set")
	}
	if res.Header.Get("Access-Control-Max-Age") != "600" {
		t.Fatalf("expected max age 600, got %s", res.Header.Get("Access-Control-Max-Age"))
	}
}

func containsHeader(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
