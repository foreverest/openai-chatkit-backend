package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared/constant"
)

type sessionCreator func(context.Context, openai.BetaChatKitSessionNewParams) (*openai.ChatSession, error)

type sessionRequest struct {
	User string `json:"user"`
}

type sessionHandler struct {
	createSession       sessionCreator
	workflowID          string
	expiresAfterSeconds int64
	rateLimitPerMinute  int64
}

func newSessionHandler(create sessionCreator, workflowID string, expiresAfterSeconds, rateLimitPerMinute int64) *sessionHandler {
	return &sessionHandler{
		createSession:       create,
		workflowID:          workflowID,
		expiresAfterSeconds: expiresAfterSeconds,
		rateLimitPerMinute:  rateLimitPerMinute,
	}
}

func newRouter(sessionHandler *sessionHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/api/chatkit/session", sessionHandler.handleSession)
	return mux
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (h *sessionHandler) handleSession(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var payload sessionRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if payload.User == "" {
		http.Error(w, "user is required", http.StatusBadRequest)
		return
	}

	debugf("creating session user=%s workflow_id=%s expires_after_seconds=%d rate_limit_per_minute=%d", payload.User, h.workflowID, h.expiresAfterSeconds, h.rateLimitPerMinute)

	ctx, cancel := context.WithTimeout(r.Context(), openaiRequestTimeout)
	defer cancel()

	params := openai.BetaChatKitSessionNewParams{
		User: payload.User,
		Workflow: openai.ChatSessionWorkflowParam{
			ID: h.workflowID,
		},
		ExpiresAfter: openai.ChatSessionExpiresAfterParam{
			Seconds: h.expiresAfterSeconds,
			Anchor:  constant.CreatedAt("").Default(),
		},
		RateLimits: openai.ChatSessionRateLimitsParam{
			MaxRequestsPer1Minute: openai.Int(h.rateLimitPerMinute),
		},
	}

	session, err := h.createSession(ctx, params)
	if err != nil {
		log.Printf("failed to create session: %v", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	debugf("session created user=%s workflow_id=%s", payload.User, h.workflowID)

	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"client_secret": session.ClientSecret}); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}
