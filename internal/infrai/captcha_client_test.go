package infrai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVerifyCaptchaRetriesRateLimit(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing bearer key")
		}
		if requests == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"valid": true}, "error": nil, "metadata": map[string]any{}})
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.BaseURL = server.URL
	client.Sleep = func(context.Context, time.Duration) error { return nil }
	result, err := client.VerifyCaptcha(context.Background(), CaptchaRequest{Token: "browser-token", Action: "checkout_login"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || requests != 2 {
		t.Fatalf("result = %+v, requests = %d", result, requests)
	}
}
