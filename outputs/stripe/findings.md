# Validate Stripe API — Blocked (No Credentials)

**Status:** Blocked pending access to Stripe API credentials.

## What’s missing

To validate the Stripe API against real behavior, PT needs:
- A Stripe API key with permission to call `GET /v1/customers` (or an agreed endpoint).
- Tooling installed locally: `curl` and `jq`.

## How to validate (once unblocked)

1. Set credentials:
   - `export STRIPE_API_KEY=...`
2. Run:
   - `curl -s -u "$STRIPE_API_KEY:" https://api.stripe.com/v1/customers | jq .`
3. Capture proof artifacts:
   - Save raw response: `outputs/stripe/customers.json`
   - Document observed fields and edge cases: `outputs/stripe/schema.md`

## Acceptance criteria (real validation)

- API responds with a `data` array of customer objects (or returns a documented auth error).
- Response shape is documented (required/optional fields, paging, error responses).

