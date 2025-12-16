# Explore: `pulse run` CLI output UX (v1)

## Goals
- Make results obvious to humans and parsable by agents/CI.
- Preserve evidence paths (screenshots/DOM/a11y dumps) on failure.
- Make drift failures actionable (show why + where to patch).

## Output format choice (v1)
- Default: **human text** with stable prefixes and a final summary.
- Optional: `--json` emits a single JSON report (same content as the text summary).

## Proposed directory layout (artifacts)
Each run writes to a timestamped directory:
`projects/pulse/outputs/runs/<run-id>/`

Suggested files:
- `report.json` (machine report; always)
- `report.txt` (captured stdout; optional)
- `flows/<flow-id>/snapshot.png` (on fail)
- `flows/<flow-id>/dom.json` (on fail)
- `flows/<flow-id>/a11y.json` (on fail)
- `patches/<flow-id>.fp.patch.toml` (when drift detected and a “best candidate” is suggested)

## Sample log: all-pass
```
$ pulse run --diff=HEAD~1..HEAD --headless
Pulse v0 (MVP)  run_id=2025-12-16T05-12-03Z  selected=2
Selected flows:
  - product_card_quickadd   (tags: area:product_card,p1)
  - settings_profile_save   (tags: area:settings,p1)

==> product_card_quickadd  PASS  1.23s
    steps=2 ok  assertions=1 ok
==> settings_profile_save  PASS  0.98s
    steps=3 ok  assertions=1 ok

Summary: pass=2 fail=0 skipped=0  duration=2.21s
Report: outputs/runs/2025-12-16T05-12-03Z/report.json
```

## Sample log: assertion failure (with snapshot path)
```
$ pulse run --flow=product_card_quickadd --headless
Pulse v0 (MVP)  run_id=2025-12-16T05-14-10Z  selected=1

==> product_card_quickadd  FAIL  3.71s
    assertion[0] visible  target=data-testid=mini-cart  timeout_ms=5000
    observed: element not found after 5000ms
    snapshot: outputs/runs/2025-12-16T05-14-10Z/flows/product_card_quickadd/snapshot.png
    dom:      outputs/runs/2025-12-16T05-14-10Z/flows/product_card_quickadd/dom.json
    a11y:     outputs/runs/2025-12-16T05-14-10Z/flows/product_card_quickadd/a11y.json

Summary: pass=0 fail=1 skipped=0  duration=3.71s
Exit: 1
```

## Sample log: fingerprint drift (with suggested patch path)
```
$ pulse run --flow=product_card_quickadd --headless
Pulse v0 (MVP)  run_id=2025-12-16T05-18-55Z  selected=1

==> product_card_quickadd  FAIL  2.44s
    step[0] click  target=fp:quick-add@v1
    drift: STRUCTURAL (hard fail)
      - semantic: role=button name="Quick Add" (matched)
      - structure: children_roles_hash mismatch (expected=2f1a… got=9c0b…)
      - locator: data-testid missing (expected=quick-add)
    suggested_patch: outputs/runs/2025-12-16T05-18-55Z/patches/product_card_quickadd.fp.patch.toml
    snapshot:        outputs/runs/2025-12-16T05-18-55Z/flows/product_card_quickadd/snapshot.png

Summary: pass=0 fail=1 skipped=0  duration=2.44s
Exit: 1
```

## Notes
- Text output should avoid multi-line stack traces by default; keep it “operator readable”.
- The JSON report should include the full drift reasons and artifact paths.
- Later: add `pulse heal --from <run_id> --flow <id>` to apply/accept a patch, but v1 only needs to *emit* the patch file path.
