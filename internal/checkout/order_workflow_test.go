package checkout

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"example.com/social-commerce/internal/infrai"
)

func TestCheckoutJSONUsesDocumentedFieldNames(t *testing.T) {
	var input Checkout
	err := json.Unmarshal([]byte(`{"order_id":"ord-1042","login":{"provider":"google","subject":"user-7","email":"buyer@example.com","captcha_token":"browser-token","ip":"203.0.113.10"},"items":["sku-coffee"],"paid":true}`), &input)
	if err != nil {
		t.Fatal(err)
	}
	if input.OrderID != "ord-1042" || input.Login.CaptchaToken != "browser-token" || input.Login.IP != "203.0.113.10" || !input.Paid {
		t.Fatalf("decoded checkout = %+v", input)
	}
}

type verifierStub struct{ valid bool }

func (v verifierStub) VerifyCaptcha(context.Context, infrai.CaptchaRequest) (infrai.CaptchaResult, error) {
	return infrai.CaptchaResult{Valid: v.valid}, nil
}

func TestPlaceOrderDecision(t *testing.T) {
	fixed := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		provider  string
		paid      bool
		wantState string
	}{
		{name: "google paid order enters fulfillment", provider: "google", paid: true, wantState: "ready_for_fulfillment"},
		{name: "github unpaid order waits", provider: "github", paid: false, wantState: "awaiting_payment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := Workflow{Captcha: verifierStub{valid: true}, Now: func() time.Time { return fixed }}
			got, err := workflow.Place(context.Background(), Checkout{
				OrderID: "ord-1042", Paid: test.paid, Items: []string{"sku-coffee"},
				Login: Login{Provider: test.provider, Subject: "user-7", Email: "buyer@example.com", CaptchaToken: "browser-token"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.State != test.wantState || got.IdempotencyKey != "ord-1042" {
				t.Fatalf("state = %q, key = %q", got.State, got.IdempotencyKey)
			}
		})
	}
}
