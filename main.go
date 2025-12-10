package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared/constant"
)

const (
	defaultAddr           = ":8080"
	openaiRequestTimeout  = 15 * time.Second
	serverShutdownTimeout = 5 * time.Second
	maxRequestBodyBytes   = 4096
	readTimeout           = 10 * time.Second
	readHeaderTimeout     = 5 * time.Second
	writeTimeout          = 15 * time.Second
	idleTimeout           = 60 * time.Second
	contentTypeJSON       = "application/json"
)

var debugEnabled = func() bool {
	v := strings.ToLower(os.Getenv("DEBUG"))
	return v == "1" || v == "true" || v == "yes"
}()

type sessionRequest struct {
	User string `json:"user"`
}

type server struct {
	createSession       func(context.Context, openai.BetaChatKitSessionNewParams) (*openai.ChatSession, error)
	workflowID          string
	expiresAfterSeconds int64
	rateLimitPerMinute  int64
}

func main() {
	addr := getEnv("ADDR", defaultAddr)

	apiKey := requireEnv("OPENAI_API_KEY")

	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	workflowID := requireEnv("CHATKIT_WORKFLOW_ID")
	expiresAfterSeconds := requireEnvInt64("CHATKIT_EXPIRES_AFTER_SECONDS")
	if expiresAfterSeconds < 0 {
		log.Fatal("CHATKIT_EXPIRES_AFTER_SECONDS must be non-negative")
	}
	rateLimitPerMinute := requireEnvInt64("CHATKIT_RATE_LIMIT_PER_MINUTE")
	if rateLimitPerMinute < 0 {
		log.Fatal("CHATKIT_RATE_LIMIT_PER_MINUTE must be non-negative")
	}

	client := openai.NewClient(opts...)

	s := &server{
		createSession: func(ctx context.Context, params openai.BetaChatKitSessionNewParams) (*openai.ChatSession, error) {
			return client.Beta.ChatKit.Sessions.New(ctx, params)
		},
		workflowID:          workflowID,
		expiresAfterSeconds: expiresAfterSeconds,
		rateLimitPerMinute:  rateLimitPerMinute,
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
	if payload.User == "" {
		http.Error(w, "user is required", http.StatusBadRequest)
		return
	}

	debugf("creating session user=%s workflow_id=%s expires_after_seconds=%d rate_limit_per_minute=%d", payload.User, s.workflowID, s.expiresAfterSeconds, s.rateLimitPerMinute)

	ctx, cancel := context.WithTimeout(r.Context(), openaiRequestTimeout)
	defer cancel()

	params := openai.BetaChatKitSessionNewParams{
		User: payload.User,
		Workflow: openai.ChatSessionWorkflowParam{
			ID: s.workflowID,
		},
		ExpiresAfter: openai.ChatSessionExpiresAfterParam{
			Seconds: s.expiresAfterSeconds,
			Anchor:  constant.CreatedAt("").Default(),
		},
		RateLimits: openai.ChatSessionRateLimitsParam{
			MaxRequestsPer1Minute: openai.Int(s.rateLimitPerMinute),
		},
	}

	session, err := s.createSession(ctx, params)
	if err != nil {
		log.Printf("failed to create session: %v", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	debugf("session created user=%s workflow_id=%s", payload.User, s.workflowID)

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

func requireEnv(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	log.Fatalf("%s is required", key)
	return ""
}

func requireEnvInt64(key string) int64 {
	v := requireEnv(key)
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		log.Fatalf("%s must be an integer: %v", key, err)
	}
	return n
}

func debugf(format string, args ...any) {
	if debugEnabled {
		log.Printf("[debug] "+format, args...)
	}
}
