package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared/constant"
)

func main() {
	addr := getEnv("ADDR", ":8080")

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("/api/chatkit/session", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var payload sessionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
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
			expiresAfter = 1200
		}
		rateLimit := payload.RateLimitPerMinute
		if rateLimit == 0 {
			rateLimit = 10
		}

		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			http.Error(w, "missing OPENAI_API_KEY", http.StatusInternalServerError)
			return
		}

		opts := []option.RequestOption{option.WithAPIKey(apiKey)}
		if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
			opts = append(opts, option.WithBaseURL(baseURL))
		}

		client := openai.NewClient(opts...)

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

		session, err := client.Beta.ChatKit.Sessions.New(r.Context(), params)
		if err != nil {
			log.Printf("failed to create session: %v", err)
			http.Error(w, "failed to create session", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"client_secret": session.ClientSecret}); err != nil {
			log.Printf("failed to write response: %v", err)
		}
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type sessionRequest struct {
	User                string `json:"user"`
	WorkflowID          string `json:"workflow_id"`
	ExpiresAfterSeconds int64  `json:"expires_after_seconds"`
	RateLimitPerMinute  int64  `json:"rate_limit_per_minute"`
}
