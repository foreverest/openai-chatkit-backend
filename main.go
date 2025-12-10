package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared/constant"
)

const (
	defaultAddr             = ":8080"
	defaultExpiresAfterSecs = int64(1200)
	defaultRateLimitPerMin  = int64(10)
	openaiRequestTimeout    = 15 * time.Second
	serverShutdownTimeout   = 5 * time.Second
	maxRequestBodyBytes     = 4096
	readTimeout             = 10 * time.Second
	readHeaderTimeout       = 5 * time.Second
	writeTimeout            = 15 * time.Second
	idleTimeout             = 60 * time.Second
	contentTypeJSON         = "application/json"
)

var debugEnabled = func() bool {
	v := strings.ToLower(os.Getenv("DEBUG"))
	return v == "1" || v == "true" || v == "yes"
}()

type sessionRequest struct {
	User                string `json:"user"`
	WorkflowID          string `json:"workflow_id"`
	ExpiresAfterSeconds int64  `json:"expires_after_seconds"`
	RateLimitPerMinute  int64  `json:"rate_limit_per_minute"`
}

type server struct {
	createSession func(context.Context, openai.BetaChatKitSessionNewParams) (*openai.ChatSession, error)
}

func main() {
	addr := getEnv("ADDR", defaultAddr)

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is required")
	}

	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	client := openai.NewClient(opts...)

	s := &server{
		createSession: func(ctx context.Context, params openai.BetaChatKitSessionNewParams) (*openai.ChatSession, error) {
			return client.Beta.ChatKit.Sessions.New(ctx, params)
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/api/chatkit/session", s.handleSession)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	go func() {
		log.Printf("listening on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	<-shutdown

	ctx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	} else {
		log.Println("server stopped")
	}
}

func (s *server) handleSession(w http.ResponseWriter, r *http.Request) {
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
	if payload.User == "" || payload.WorkflowID == "" {
		http.Error(w, "user and workflow_id are required", http.StatusBadRequest)
		return
	}
	if payload.ExpiresAfterSeconds < 0 {
		http.Error(w, "expires_after_seconds must be non-negative", http.StatusBadRequest)
		return
	}
	if payload.RateLimitPerMinute < 0 {
		http.Error(w, "rate_limit_per_minute must be non-negative", http.StatusBadRequest)
		return
	}

	expiresAfter := payload.ExpiresAfterSeconds
	if expiresAfter == 0 {
		expiresAfter = defaultExpiresAfterSecs
	}
	rateLimit := payload.RateLimitPerMinute
	if rateLimit == 0 {
		rateLimit = defaultRateLimitPerMin
	}
	debugf("creating session user=%s workflow_id=%s expires_after_seconds=%d rate_limit_per_minute=%d", payload.User, payload.WorkflowID, expiresAfter, rateLimit)

	ctx, cancel := context.WithTimeout(r.Context(), openaiRequestTimeout)
	defer cancel()

	params := openai.BetaChatKitSessionNewParams{
		User: payload.User,
		Workflow: openai.ChatSessionWorkflowParam{
			ID: payload.WorkflowID,
		},
		ExpiresAfter: openai.ChatSessionExpiresAfterParam{
			Seconds: expiresAfter,
			Anchor:  constant.CreatedAt("").Default(),
		},
		RateLimits: openai.ChatSessionRateLimitsParam{
			MaxRequestsPer1Minute: openai.Int(rateLimit),
		},
	}

	session, err := s.createSession(ctx, params)
	if err != nil {
		log.Printf("failed to create session: %v", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	debugf("session created user=%s workflow_id=%s", payload.User, payload.WorkflowID)

	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"client_secret": session.ClientSecret}); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func debugf(format string, args ...any) {
	if debugEnabled {
		log.Printf("[debug] "+format, args...)
	}
}
