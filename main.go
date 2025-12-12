package main

import (
	"context"
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

	sessionHandler := newSessionHandler(
		func(ctx context.Context, params openai.BetaChatKitSessionNewParams) (*openai.ChatSession, error) {
			return client.Beta.ChatKit.Sessions.New(ctx, params)
		},
		workflowID,
		expiresAfterSeconds,
		rateLimitPerMinute,
	)

	mux := newRouter(sessionHandler)

	corsPolicy := newCORSPolicy(requireEnv("CORS_ALLOWED_ORIGINS"))

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           withCORS(corsPolicy, mux),
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
