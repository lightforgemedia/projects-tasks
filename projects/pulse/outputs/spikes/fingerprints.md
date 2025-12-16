# Spike: Fingerprint model + drift policy (Pulse)

## Goals
- Define a **selector-light** fingerprint that can survive minor UI churn.
- Classify drift as **structural** (hard fail) vs **cosmetic** (tolerable) deterministically.
- Enable “agentic healing”: when a fingerprint fails, the agent can re-align to a new element and propose an updated fingerprint.

## Fingerprint v1 (data model)
A fingerprint captures an element’s *semantic identity* plus a *shape check*.

Recommended fields:
- `id`: stable name, e.g. `mini-cart`
- `version`: integer (increment on structural changes)
- `semantic`:
  - `role`: ARIA role (e.g. `button`, `dialog`, `textbox`)
  - `name`: accessible name (string)
- `locators` (ranked, optional):
  - `testid`: `data-testid` value
  - `css`: last-resort selector (discouraged; optional)
- `structure`:
  - `children_roles_hash`: hash of direct children roles (ordered)
  - `attrs_hash`: hash of a small allowlist (e.g. `aria-*`, `data-*` stable attrs)
- `text` (optional):
  - `inner_text_hash`: hash of normalized text (trim/collapse whitespace)
  - `max_len`: limit used when hashing (prevents huge payloads)
- `layout` (optional, coarse):
  - `bbox_rel`: element bbox relative to viewport (x/y/w/h as ratios)
  - `tolerance`: `{pos_pct, size_pct}`

Minimal on-wire representation (returned by `__pulse.ListInteractive()`):
```json
{
  "fingerprint": "fp:mini-cart@v1",
  "role": "dialog",
  "name": "Mini cart",
  "hints": ["data-testid=mini-cart"],
  "rect": {"x": 812, "y": 72, "w": 360, "h": 640}
}
```

## Hashing choices
Keep hashing simple and reproducible:
- Use `sha256` for stable, portable hashes.
- Normalize text before hashing:
  - trim
  - collapse runs of whitespace
  - optional: lowercase (only if the product treats casing as cosmetic)

## Drift policy (classification)
When a step/assertion references `fp:<name>@vN`, resolution can fail in two ways:
1) No candidate elements match the semantic identity well enough
2) A candidate exists but fails drift checks

### Structural drift (hard fail)
Fail the run (requires fingerprint update) when any of these change:
- Role changes (`button` → `link`)
- Accessible name changes meaningfully (configurable; default hard fail)
- `children_roles_hash` mismatch (shape changed)
- Required stable locator disappears (e.g. expected `data-testid` missing)

### Cosmetic drift (tolerable)
Allow the run to proceed (log a warning) when changes are within tolerance:
- Minor text changes (punctuation/whitespace) if `text` is marked optional
- Bounding box movement within tolerance (e.g. position ±20%, size ±20%)
- Additional attributes/styles that don’t affect allowlisted structural attrs

Recommended default tolerances:
- `layout.pos_pct`: 0.20
- `layout.size_pct`: 0.20
- Text: whitespace-only changes are cosmetic; content changes are structural unless explicitly configured otherwise.

## Scenarios (examples)
| Change | Example | Classification | Rationale |
|---|---|---|---|
| Button text “Save” → “Save changes” | label tweak | Structural (default) | Meaning may change; requires explicit opt-in to treat as cosmetic |
| Button becomes icon-only but keeps `aria-label="Save"` | UI redesign | Cosmetic/OK | Semantic identity preserved |
| Modal adds a new section | DOM shape change | Structural | Likely affects user flow and assertions |
| Element moves due to responsive layout | CSS tweak | Cosmetic | Location drift within bounds |
| `data-testid` removed | selector removed | Structural | Breaks stable locator contract |

## Healing flow (agent-assisted)
When a fingerprint fails:
1. Controller records failure: expected `fp:X@vN`, observed candidates from `ListInteractive()`.
2. Agent selects best candidate by semantic similarity (role/name/testid proximity).
3. Agent performs the action on the candidate and proposes:
   - Update flow target to the candidate’s new fingerprint, **or**
   - Bump fingerprint version (`fp:X@v2`) with updated `structure`/`locators`.

The important invariant: **healing is explicit** and produces a patch (no silent auto-mutation).

## Output expectations for later implementation
- Runtime should compute/return `fp:<name>@vN` deterministically for interactive elements.
- Flow runner should emit drift classification in results:
  - `structural_drift` → fail
  - `cosmetic_drift` → warn + continue
