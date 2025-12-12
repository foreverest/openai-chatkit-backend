package main

import (
	"net/http"
	"strings"
)

type corsPolicy struct {
	allowAll bool
	origins  map[string]struct{}
}

func newCORSPolicy(allowedOrigins string) corsPolicy {
	if allowedOrigins == "" || allowedOrigins == "*" {
		return corsPolicy{allowAll: true}
	}

	policy := corsPolicy{origins: make(map[string]struct{})}
	for _, origin := range strings.Split(allowedOrigins, ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		policy.origins[origin] = struct{}{}
	}
	if len(policy.origins) == 0 {
		policy.allowAll = true
	}
	return policy
}

func (p corsPolicy) allow(origin string) (string, bool) {
	if p.allowAll {
		return "*", true
	}
	_, ok := p.origins[origin]
	return origin, ok
}

func withCORS(policy corsPolicy, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		allowedOrigin, ok := policy.allow(origin)
		if !ok {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}

		headers := w.Header()
		headers.Set("Access-Control-Allow-Origin", allowedOrigin)
		headers.Add("Vary", "Origin")

		if r.Method == http.MethodOptions {
			headers.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			headers.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			headers.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
