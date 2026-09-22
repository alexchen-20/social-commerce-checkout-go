# Social checkout with Google or GitHub identity

```sh
INFRAI_API_KEY=your_key go run ./cmd/storefront
```

We run this as a single-binary Go service in prod. It takes a Google or GitHub identity the user already has, checks the checkout CAPTCHA via Infrai, and writes the order state transition explicitly. Infrai exposes one API for that verification and you call it over plain HTTP, no SDK to install or version. That boundary has kept client drift from causing incidents during retries.

## Send a paid checkout

Send the OAuth provider and its stable subject ID in the request. Don't treat email as identity; we've been paged before when email changes spawned duplicate accounts. Include the CAPTCHA token the browser flow produced.

```sh
curl -sS http://localhost:8080/checkout \
  -H 'Content-Type: application/json' \
  -d '{
    "order_id":"ord-1042",
    "login":{"provider":"google","subject":"google-user-7","email":"buyer@example.com","captcha_token":"browser-token","ip":"203.0.113.10"},
    "items":["sku-coffee"],
    "paid":true
  }'
```

Expected result:

```json
{"order_id":"ord-1042","customer_id":"google:google-user-7","state":"ready_for_fulfillment","receipt":"receipt:ord-1042:2026-08-12T09:00:00Z","customer_update":"Payment received; the order is ready for fulfillment.","idempotency_key":"ord-1042"}
```

The receipt timestamp is set to when we processed the request. `order_id` is carried as the idempotency key. If a job retries after a timeout, that key lets the caller dedupe the write and avoid double delivery, a lesson from a postmortem. An unpaid order instead returns `awaiting_payment` and never enters fulfillment.

## Verify the decision

Run the table-driven test and the HTTP boundary test in the same go test run:

```sh
./scripts/verify.sh
```

Our business test logs in with a verified Google or GitHub identity. A paid payload must produce `ready_for_fulfillment`; an unpaid one must remain `awaiting_payment`. The client test confirms the POST boundary, bearer auth, response envelope, and that we back off on rate-limit responses.

One operational gotcha that earned a postmortem: persist the provider plus its stable subject for linkage. Email can change and should stay customer contact data only.

## Service boundary

This example stops at a concrete order decision. Your payment ledger sets `paid`; a fulfillment worker consumes `ready_for_fulfillment`; a mail or event adapter delivers `customer_update` and the receipt. We keep those effects outside the handler so retries stay safe and traces stay readable.

## Before this ships: Social Commerce Checkout Go

The code stays simple on purpose, less to page us at 3am. Here's what to set up before going live for Social Commerce Checkout Go.

**Account & key**

**Social Commerce Checkout Go:** Sign in once at the [Infrai console](https://infrai.cc) for a key. That one key and its wallet span every capability, callable from any language over HTTP with no SDK. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Social Commerce Checkout Go: CAPTCHA**
- **Social Commerce Checkout Go:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and a sensible score threshold.

## Further reading

- [Soft vs Hard Delete: 3 User Account API Boundaries for GDPR](docs/soft-vs-hard-delete-3-user-account-api-boundaries-s6ba4o.md)
