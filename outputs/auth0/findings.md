# Validate Auth0 API — Findings

**Status:** Out of scope for current MVP.

## Decision

Auth0 integration is **not required** for the current project scope. This means we will not validate Auth0 tenant behavior, JWT claims, or token flows at this time.

This task is being closed to avoid blocking the `prove` phase on an unused dependency.

## If Auth0 is reintroduced later (what must be validated)

### Required inputs (configuration/secrets)
- `AUTH0_DOMAIN` (tenant domain)
- `AUTH0_AUDIENCE` (API audience, if using access tokens for APIs)
- `AUTH0_CLIENT_ID`
- One of:
  - `AUTH0_CLIENT_SECRET` (confidential client), or
  - PKCE flow parameters (public client)

### Validation steps (real behavior)
1. Fetch OIDC config:
   - `GET https://$AUTH0_DOMAIN/.well-known/openid-configuration`
2. Fetch JWKS and ensure key rotation compatibility:
   - follow `jwks_uri` from the OIDC config
3. Obtain a real token via the intended flow (client credentials, auth code + PKCE, etc).
4. Verify JWT:
   - `iss`, `aud`, `exp`, `iat`, `sub`
   - app-specific claims (roles/scopes)
5. Run integration tests:
   - token acquisition succeeds
   - JWT validates against JWKS
   - expected claims/scopes exist
   - failure cases are handled (expired token, wrong audience, missing scope)

### Mock policy

Mocks are only allowed after:
- a spike captures real API responses and token shapes
- mock fixtures are derived from that captured data
- an explicit integration task exists to remove/retire mocks

