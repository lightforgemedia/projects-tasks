package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestEngineV2Conformance_StatusAndNext_JSONParity(t *testing.T) {
	type scenario struct {
		name       string
		wf         string
		nextArgs   []string
		setupStore func(t *testing.T, store *pt.StoreClient)
		assertV1   func(t *testing.T, next map[string]any, status map[string]any)
	}

	baseDoD := pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}

	scenarios := []scenario{
		{
			name: "needs_review_prioritized",
			wf: `
name = "wf"

[phase_assignment]
label_prefix = "phase:"
default_phase = "build"

[[phases]]
id = "build"
name = "Build"
order = 1
`,
			nextArgs: []string{"--json"},
			setupStore: func(t *testing.T, store *pt.StoreClient) {
				manifest := pt.Manifest{
					Tasks: []pt.Task{
						{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "code:a", DoD: baseDoD},
						{Title: "B", Template: "backend_endpoint", Role: "dev", Artifact: "code:b", DoD: baseDoD},
					},
				}
				if _, err := store.Sync(t.Context(), manifest); err != nil {
					t.Fatalf("sync: %v", err)
				}
				if err := store.UpdateIssue(t.Context(), "pt-1", "needs_review", ""); err != nil {
					t.Fatalf("update: %v", err)
				}
				_ = store.AddLabels(t.Context(), "pt-1", "phase:build")
				_ = store.AddLabels(t.Context(), "pt-2", "phase:build")
			},
			assertV1: func(t *testing.T, next map[string]any, _ map[string]any) {
				if next["mode"] != "REVIEW" {
					t.Fatalf("mode=%v, want REVIEW", next["mode"])
				}
			},
		},
		{
			name: "checkpoint_kickoff_recommended",
			wf: `
name = "wf"

[phase_assignment]
label_prefix = "phase:"
default_phase = "prove"

[[phases]]
id = "prove"
name = "Prove"
order = 1
`,
			nextArgs: []string{"--json"},
			setupStore: func(t *testing.T, store *pt.StoreClient) {
				manifest := pt.Manifest{
					Tasks: []pt.Task{
						{Title: "[Phase Pre] Prove", Template: "discovery", Role: "planner", Artifact: "review:prove:pre", DoD: baseDoD},
						{Title: "Work Task", Template: "backend_endpoint", Role: "dev", Artifact: "code:work", DoD: baseDoD},
					},
				}
				if _, err := store.Sync(t.Context(), manifest); err != nil {
					t.Fatalf("sync: %v", err)
				}
				_ = store.AddLabels(t.Context(), "pt-1", "phase:prove", "checkpoint:required", "checkpoint:pre")
				_ = store.AddLabels(t.Context(), "pt-2", "phase:prove")
			},
			assertV1: func(t *testing.T, next map[string]any, _ map[string]any) {
				if next["mode"] != "REVIEW" {
					t.Fatalf("mode=%v, want REVIEW", next["mode"])
				}
			},
		},
		{
			name: "strict_soft_gate_blocks_later_phase",
			wf: `
name = "wf"

[phase_assignment]
label_prefix = "phase:"

[[phases]]
id = "prove"
name = "Prove"
order = 1
[phases.gate]
type = "soft"
condition = "has_comment:user-approved"

[[phases]]
id = "build"
name = "Build"
order = 2
`,
			nextArgs: []string{"--json", "--strict"},
			setupStore: func(t *testing.T, store *pt.StoreClient) {
				manifest := pt.Manifest{
					Tasks: []pt.Task{
						{Title: "Prove Task", Template: "spike", Role: "dev", Artifact: "doc:prove", DoD: baseDoD},
						{Title: "Build Task", Template: "backend_endpoint", Role: "dev", Artifact: "code:build", DoD: baseDoD},
					},
				}
				if _, err := store.Sync(t.Context(), manifest); err != nil {
					t.Fatalf("sync: %v", err)
				}
				_ = store.AddLabels(t.Context(), "pt-1", "phase:prove")
				_ = store.AddLabels(t.Context(), "pt-2", "phase:build")
				// Close the prove task, but do not add the required comment tag.
				if err := store.UpdateIssue(t.Context(), "pt-1", "closed", ""); err != nil {
					t.Fatalf("close: %v", err)
				}
			},
			assertV1: func(t *testing.T, next map[string]any, _ map[string]any) {
				if next["mode"] != "BLOCKED" {
					t.Fatalf("mode=%v, want BLOCKED", next["mode"])
				}
			},
		},
		{
			name: "deps_blocked_no_claimable_work",
			wf: `
name = "wf"

[phase_assignment]
label_prefix = "phase:"
default_phase = "build"

[[phases]]
id = "build"
name = "Build"
order = 1
`,
			nextArgs: []string{"--json"},
			setupStore: func(t *testing.T, store *pt.StoreClient) {
				manifest := pt.Manifest{
					Tasks: []pt.Task{
						{Title: "Dep", Template: "backend_endpoint", Role: "dev", Artifact: "code:dep", DoD: baseDoD},
						{Title: "Blocked", Template: "backend_endpoint", Role: "dev", Artifact: "code:blocked", Deps: []string{"Dep"}, DoD: baseDoD},
					},
				}
				if _, err := store.Sync(t.Context(), manifest); err != nil {
					t.Fatalf("sync: %v", err)
				}
				_ = store.AddLabels(t.Context(), "pt-1", "phase:build")
				_ = store.AddLabels(t.Context(), "pt-2", "phase:build")
				// Make only pt-2 open; pt-1 is in_progress, so pt-2 is deps-blocked and no other open tasks exist.
				if err := store.UpdateIssue(t.Context(), "pt-1", "in_progress", "dev"); err != nil {
					t.Fatalf("update pt-1: %v", err)
				}
			},
			assertV1: func(t *testing.T, next map[string]any, _ map[string]any) {
				if next["mode"] != "BLOCKED" {
					t.Fatalf("mode=%v, want BLOCKED", next["mode"])
				}
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			_, store := setupStoreEnv(t)

			// Stable DoD path in output (avoid accidental coupling to repo root).
			dodPath := filepath.Join(t.TempDir(), "PROJECT_DOD.md")
			if err := os.WriteFile(dodPath, []byte("# DoD\n"), 0o644); err != nil {
				t.Fatalf("write dod: %v", err)
			}
			t.Setenv("PT_PROJECT_DOD", dodPath)

			wfPath := filepath.Join(t.TempDir(), "wf.toml")
			if err := os.WriteFile(wfPath, []byte(sc.wf), 0o644); err != nil {
				t.Fatalf("write wf: %v", err)
			}

			sc.setupStore(t, store)

			// V1 (default)
			t.Setenv("PT_ENGINE", "")
			nextV1Args := append([]string{"--db", os.Getenv("PT_DB"), "--workflow", wfPath}, sc.nextArgs...)
			nextV1 := runCmdNextJSON(t, nextV1Args)
			statusV1 := runCmdWorkflowStatusJSON(t, []string{"--db", os.Getenv("PT_DB"), "--workflow", wfPath, "--json"})
			sc.assertV1(t, nextV1, statusV1)

			// V2 (flagged)
			t.Setenv("PT_ENGINE", "v2")
			nextV2Args := append([]string{"--db", os.Getenv("PT_DB"), "--workflow", wfPath}, sc.nextArgs...)
			nextV2 := runCmdNextJSON(t, nextV2Args)
			statusV2 := runCmdWorkflowStatusJSON(t, []string{"--db", os.Getenv("PT_DB"), "--workflow", wfPath, "--json"})

			if !reflect.DeepEqual(nextV1, nextV2) {
				t.Fatalf("next JSON mismatch v1 vs v2\nv1=%v\nv2=%v", nextV1, nextV2)
			}
			if !reflect.DeepEqual(statusDigest(statusV1), statusDigest(statusV2)) {
				t.Fatalf("status digest mismatch v1 vs v2\nv1=%v\nv2=%v", statusDigest(statusV1), statusDigest(statusV2))
			}
		})
	}
}

func statusDigest(status map[string]any) map[string]any {
	out := map[string]any{}
	if wf, ok := status["workflow"].(map[string]any); ok {
		out["workflow"] = map[string]any{
			"name": wf["name"],
		}
	}
	if phases, ok := status["phases"].([]any); ok {
		var digest []any
		for _, p := range phases {
			ps, _ := p.(map[string]any)
			phase, _ := ps["phase"].(map[string]any)
			digest = append(digest, map[string]any{
				"id":           phase["id"],
				"order":        phase["order"],
				"total_tasks":  ps["total_tasks"],
				"closed_tasks": ps["closed_tasks"],
				"is_blocked":   ps["is_blocked"],
				"block_reason": ps["block_reason"],
				"is_current":   ps["is_current"],
			})
		}
		out["phases"] = digest
	}
	if sn, ok := status["suggested_next"].(map[string]any); ok {
		out["suggested_next"] = map[string]any{"id": sn["id"]}
	} else {
		out["suggested_next"] = nil
	}
	out["next_reason"] = status["next_reason"]
	return out
}

func runCmdWorkflowStatusJSON(t *testing.T, args []string) map[string]any {
	t.Helper()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmdWorkflowStatus(args)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("cmdWorkflowStatus: %v", err)
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%s", err, buf.String())
	}
	return out
}
