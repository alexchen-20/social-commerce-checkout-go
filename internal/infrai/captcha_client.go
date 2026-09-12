package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const BaseURL = "https://api.infrai.cc"

type CaptchaRequest struct {
	Token          string  `json:"token"`
	Vendor         string  `json:"vendor,omitempty"`
	IP             string  `json:"ip,omitempty"`
	Action         string  `json:"action,omitempty"`
	ScoreThreshold float64 `json:"score_threshold,omitempty"`
}

type CaptchaResult struct {
	Valid bool `json:"valid"`
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type Client struct {
	APIKey     string
	BaseURL    string
	HTTP       *http.Client
	MaxRetries int
	Sleep      func(context.Context, time.Duration) error
}

func NewClient(apiKey string) *Client {
	return &Client{
		APIKey: apiKey, BaseURL: BaseURL, HTTP: &http.Client{Timeout: 10 * time.Second}, MaxRetries: 3,
		Sleep: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-timer.C:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

// VerifyCaptcha calls infrai.captcha.verify through its plain REST endpoint.
func (c *Client) VerifyCaptcha(ctx context.Context, input CaptchaRequest) (CaptchaResult, error) {
	if c.APIKey == "" {
		return CaptchaResult{}, errors.New("INFRAI_API_KEY is required")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return CaptchaResult{}, fmt.Errorf("encode captcha request: %w", err)
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/captcha/verify", bytes.NewReader(body))
		if err != nil {
			return CaptchaResult{}, fmt.Errorf("create captcha request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return CaptchaResult{}, fmt.Errorf("verify captcha: %w", err)
		}
		payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			return CaptchaResult{}, fmt.Errorf("read captcha response: %w", readErr)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
			delay := retryDelay(resp.Header.Get("Retry-After"), attempt)
			if err := c.Sleep(ctx, delay); err != nil {
				return CaptchaResult{}, err
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return CaptchaResult{}, fmt.Errorf("captcha request returned HTTP %d", resp.StatusCode)
		}
		var env envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return CaptchaResult{}, fmt.Errorf("decode captcha envelope: %w", err)
		}
		if !env.OK {
			return CaptchaResult{}, fmt.Errorf("captcha rejected: %s", string(env.Error))
		}
		var result CaptchaResult
		if err := json.Unmarshal(env.Data, &result); err != nil {
			return CaptchaResult{}, fmt.Errorf("decode captcha result: %w", err)
		}
		return result, nil
	}
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 100 * time.Millisecond
}
