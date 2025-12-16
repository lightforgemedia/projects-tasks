# Spike: Micro-flow TOML schema v1 (Pulse)

## Goals
- Define an ergonomic, strict TOML schema for “micro-flows” (small, atomic UI journeys).
- Define deterministic validation with **actionable** error messages (path + reason).
- Keep v1 intentionally small (enough to run + assert + snapshot on failure).

## File shape (v1)

```toml
id = "product_card_quickadd"   # required, unique, snake_case
version = 1                    # optional (default 1)
tags = ["catalog", "cart", "p1", "area:product_card"]
description = "Quick add from product card into mini-cart"

[preconditions]                # optional
route = "/products?query=socks"
login = "fixture:buyer_basic"

[[steps]]                      # required, >=1
action = "click"               # click|type|press|wait
target = "role=button[name='Quick Add']"

[[steps]]
action = "type"
target = "role=textbox[name='Quantity']"
value = "1"

[[assertions]]                 # optional (but recommended), >=0
type = "visible"               # visible|hidden|text_contains|url_is
target = "data-testid=mini-cart"
fingerprint = "fp:mini-cart@v1"  # optional (for drift policy later)
```

## Example flows (3)

### 1) Product quick add
```toml
id = "product_card_quickadd"
version = 1
tags = ["catalog", "cart", "p1", "area:product_card"]

[preconditions]
route = "/products?query=socks"
login = "fixture:buyer_basic"

[[steps]]
action = "click"
target = "role=button[name='Quick Add']"

[[assertions]]
type = "visible"
target = "data-testid=mini-cart"
```

### 2) Login happy path (UI-level)
```toml
id = "auth_login_basic"
version = 1
tags = ["auth", "p0", "area:auth"]

[preconditions]
route = "/login"

[[steps]]
action = "type"
target = "role=textbox[name='Email']"
value = "user@example.com"

[[steps]]
action = "type"
target = "role=textbox[name='Password']"
value = "correct-horse-battery-staple"

[[steps]]
action = "click"
target = "role=button[name='Sign in']"

[[assertions]]
type = "url_is"
value = "/dashboard"
```

### 3) Open modal and save
```toml
id = "settings_profile_save"
version = 1
tags = ["settings", "p1", "area:settings"]

[preconditions]
route = "/settings/profile"
login = "fixture:buyer_basic"

[[steps]]
action = "click"
target = "role=button[name='Edit profile']"

[[steps]]
action = "type"
target = "role=textbox[name='Display name']"
value = "Agent Test"

[[steps]]
action = "click"
target = "role=button[name='Save']"

[[assertions]]
type = "text_contains"
target = "data-testid=toast"
value = "Saved"
```

## Locators (target)
`target` is a **single string** with one of:
- `role=<role>[name='<accessible name>']` (preferred)
- `data-testid=<value>`
- `css=<selector>` (escape hatch; discouraged)

Rules:
- `role=` requires a role (e.g. `button`, `textbox`, `link`).
- `name='…'` is optional but strongly recommended for disambiguation.
- Exactly one locator form per `target`.

## Steps (v1)

Required fields:
- `action` (enum): `click`, `type`, `press`, `wait`

Action-specific fields:
- `click`: requires `target`
- `type`: requires `target` and `value` (string)
- `press`: requires `key` (e.g. `Enter`, `Escape`), optional `target`
- `wait`: supports `ms` (int) **or** `until` (enum: `dom_ready`, `network_idle`)

Optional fields for all steps:
- `timeout_ms` (int, default 5000)
- `retry` (int, default 0)
- `note` (string) – why this step exists

## Assertions (v1)
Supported `type` values:
- `visible` / `hidden` (requires `target`)
- `text_contains` (requires `target` and `value`)
- `url_is` (requires `value`)

Optional:
- `fingerprint` (string): `fp:<name>@vN` (used later for drift classification)

## Validation & error messages (must be deterministic)
Validation errors must include:
- **path** (TOML location-like): `id`, `steps[0].action`, `assertions[1].target`
- **reason** (what’s wrong)
- **allowed** values (for enums) when applicable

Canonical messages (examples):
- `id: required`
- `id: must be snake_case (got "Product Card")`
- `steps: required (must contain at least 1 step)`
- `steps[0].action: unsupported value "tap" (allowed: click,type,press,wait)`
- `steps[1].value: required for action "type"`
- `assertions[0].type: unsupported value "exists" (allowed: visible,hidden,text_contains,url_is)`
- `steps[0].target: invalid locator (must start with one of: role=, data-testid=, css=)`

## Tagging & naming rules (v1)
- `id`: `snake_case`, unique per repo; prefer `{area}_{microflow}`.
- `tags`: include `area:<name>` for impact mapping, plus priority (`p0|p1|p2`).
- Keep flows atomic: a single intent (e.g., “quick add”), not a whole checkout.

## Invalid examples to include in tests later (5)
These should be used as fixtures when implementing the parser/validator:
1. Missing `id`
2. Empty `steps`
3. Unsupported step action (`tap`)
4. `type` step missing `value`
5. Assertion `text_contains` missing `value`

### Invalid example snippets + expected errors

1) Missing `id`
```toml
version = 1
[[steps]]
action = "click"
target = "role=button[name='Save']"
```
Expected: `id: required`

2) Empty `steps`
```toml
id = "empty_steps"
version = 1
```
Expected: `steps: required (must contain at least 1 step)`

3) Unsupported action
```toml
id = "bad_action"
[[steps]]
action = "tap"
target = "role=button[name='Save']"
```
Expected: `steps[0].action: unsupported value "tap" (allowed: click,type,press,wait)`

4) `type` missing value
```toml
id = "type_missing_value"
[[steps]]
action = "type"
target = "role=textbox[name='Email']"
```
Expected: `steps[0].value: required for action "type"`

5) `text_contains` missing value
```toml
id = "assert_missing_value"
[[steps]]
action = "click"
target = "role=button[name='Save']"
[[assertions]]
type = "text_contains"
target = "data-testid=toast"
```
Expected: `assertions[0].value: required for type "text_contains"`

Next implementation tasks:
- `pulse-6` (parse TOML into Go structs) should enforce these rules.
- `pulse-8` (runner skeleton) should consume the validated model.
