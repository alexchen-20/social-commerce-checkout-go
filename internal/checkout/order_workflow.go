package checkout

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/social-commerce/internal/infrai"
)

type CaptchaVerifier interface {
	VerifyCaptcha(context.Context, infrai.CaptchaRequest) (infrai.CaptchaResult, error)
}

type Login struct {
	Provider     string `json:"provider"`
	Subject      string `json:"subject"`
	Email        string `json:"email"`
	CaptchaToken string `json:"captcha_token"`
	IP           string `json:"ip"`
}

type Checkout struct {
	OrderID string   `json:"order_id"`
	Login   Login    `json:"login"`
	Items   []string `json:"items"`
	Paid    bool     `json:"paid"`
}

type Result struct {
	OrderID        string `json:"order_id"`
	CustomerID     string `json:"customer_id"`
	State          string `json:"state"`
	Receipt        string `json:"receipt"`
	CustomerUpdate string `json:"customer_update"`
	IdempotencyKey string `json:"idempotency_key"`
}

type Workflow struct {
	Captcha CaptchaVerifier
	Now     func() time.Time
}

func (w Workflow) Place(ctx context.Context, input Checkout) (Result, error) {
	if input.OrderID == "" || input.Login.Subject == "" || input.Login.Email == "" || len(input.Items) == 0 {
		return Result{}, errors.New("order_id, social identity, email, and items are required")
	}
	provider := strings.ToLower(input.Login.Provider)
	if provider != "google" && provider != "github" {
		return Result{}, errors.New("provider must be google or github")
	}
	verified, err := w.Captcha.VerifyCaptcha(ctx, infrai.CaptchaRequest{
		Token: input.Login.CaptchaToken, IP: input.Login.IP, Action: "checkout_login", ScoreThreshold: 0.7,
	})
	if err != nil {
		return Result{}, fmt.Errorf("verify checkout login: %w", err)
	}
	if !verified.Valid {
		return Result{}, errors.New("checkout login was not verified")
	}
	state := "awaiting_payment"
	update := "Order accepted; payment is required before fulfillment."
	if input.Paid {
		state = "ready_for_fulfillment"
		update = "Payment received; the order is ready for fulfillment."
	}
	now := time.Now().UTC()
	if w.Now != nil {
		now = w.Now().UTC()
	}
	return Result{
		OrderID: input.OrderID, CustomerID: provider + ":" + input.Login.Subject, State: state,
		Receipt:        fmt.Sprintf("receipt:%s:%s", input.OrderID, now.Format(time.RFC3339)),
		CustomerUpdate: update, IdempotencyKey: input.OrderID,
	}, nil
}
