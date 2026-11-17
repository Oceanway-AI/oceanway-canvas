# Studio unified context baseline — 2026-11-17

Scope: WP-STUDIO-018 ST0 and preparation for ST1. Source baseline is
`dcfaf93bf96e84ba0212c5edc70f9f58e685ec0d` (`oceanway-canvas`, the current Studio
repository). This is a source and local fixture audit, not a deployment or
cross-account team acceptance.

## Published dependency

The current mainline already adopts `platformclient/v1.0.0-rc.5`; keep that version
and its newly merged catalog/recovery behavior. The earlier rc.4 integration
candidate must not downgrade it. Remote tag verification and `go mod download`
resolve rc.5 to `39ebdee5cd9894c9e39993aad372217e1ceb30dc`, module sum
`h1:zg6s6UeaQ3b+6i7hfxbQZP/OD8Elty//cGucrqkJD9I=` and go.mod sum
`h1:bNGb+1LnSL/X/hNdSFqXzKE5olaJAhYrJxsEwBSXZZQ=`. No local SDK replace is used.

`account.go`, `workspaces.go`, `workspace_selection.go` and
`console_navigation.go` are byte-identical to the published rc.4 used to deliver
Platform P1–P4. The source manifest now matches the actual dependency. Its
`sourceAdopted` flag describes the dependency only; `unifiedContextAdopted` and
`runtimeAdopted` remain false for this work package.

## Existing paths and remaining seams

| Boundary | Actual source | What the team integration must preserve or add |
| --- | --- | --- |
| User identity | `backend/internal/app/studio_platform.go` | Real Platform session to local user projection; never replace the actor with a team pseudo-user. |
| Account summary | `backend/internal/app/studio_account_summary.go` | Currently calls the implicit personal summary and rejects team results. ST1 must resolve explicit/default/object context before reading the summary. |
| Canvas ownership | `backend/internal/model/models_project.go`, `backend/internal/repository/canvas_project_read.go`, `backend/internal/handler/user_data.go` | Canvas data is authorized with `user_id`; no team ownership column or membership boundary exists. Existing personal canvases must remain personal. |
| Durable edits | `backend/internal/canvas/operations.go`, `backend/internal/repository/canvas_mutation_receipts.go` | Reuse mutation IDs, payload digests, revisions, field preconditions and persisted receipts. Separate the shared object's space from the real author of each edit. |
| Editing leases/history | `backend/internal/canvas/edit_leases.go`, `backend/internal/model/models_canvas_sync.go`, `models_canvas_history.go` | Existing keys are user/canvas scoped. A team scope must serialize writes to the same object while preserving actor and session ownership. |
| Live updates | `backend/internal/canvas/realtime.go`, `backend/internal/handler/canvas_realtime.go` | Redis rooms are user/canvas scoped. The handler rechecks the user session and canvas every 15 seconds, but not team membership. Team broadcasts need fresh membership/generation checks and shared object rooms. |
| Browser recovery | `web/src/services/user-data-sync.ts`, `web/src/lib/user-scope.ts` | Preserve the operation journal, confirmed baselines, epoch fences and recovery copies. Current cache scope is the user; ST1/ST2 must bind workspace and membership generation without moving personal drafts. |
| Generation/materials | `backend/internal/app/studio_generation.go`, `studio_media_price.go`, `studio_materials*.go` | Generation currently rejects team intent. Task workspace fields alone do not make team execution or shared resources available. Preserve original task identity and reject unsupported team requests without personal fallback. |
| Common controls | `web/src/components/layout/studio-account-bar.tsx` | Old combined account bar remains. ST1 must consume the accepted independent product switcher, footer identity and account panel without replacing the product shell. |

## Local verification

On the above actual mainline with published rc.5:

```sh
cd backend
GOWORK=off go test ./internal/canvas ./internal/handler \
  -run 'Test(CanvasSaveRevisionContract|ApplyCanvasOperations|CanvasEditLease|CanvasRealtimePresenceAndScopeIsolation|StudioAccountSummaryUsesCurrentPlatformPayer|StudioRoleRevocationAndRealtimeRevalidation)' \
  -count=1 -json
```

28 top-level tests passed; no failed or skipped test events. These cover personal
isolation, revision conflicts, disjoint edits, repeated mutation receipts,
deleted-object protection, lease ownership and existing session revalidation.
They use the repository's local fixtures; they do not prove PostgreSQL
multi-process behavior or two real team accounts editing one canvas.

## Cases still required before ST0/ST1/ST2 closure

### Executed host acceptance probes

`red-host-cases.patch` and `red-host-cases.json` preserve an executable failing
baseline on `3e2ba309bc6ec3b8d438fc7f0eea378847050295`. Apply with
`git apply --unidiff-zero` in an isolated checkout and run the recorded Go
command from its `backend/` directory. Four leaf cases fail as
expected (three entry cases and one shared-canvas case); no skips or fake green
assertions were added to the ordinary regression suite:

- An explicit `workspace_id=team_alpha` account request reads personal funds.
- A new entry does not read the confirmed team default and returns personal funds.
- An unavailable default also returns a successful personal summary.
- The original canvas operations route accepts a team intent but persists a
  creator-only object; the other effective member gets 404 for that same ID.

These are candidate host acceptance cases for ST1/ST2. The query intent is not
an authority credential. Actual team context and generation still require the
published SDK and provider checks. Fixtures use two synthetic sessions, the
real Studio route/service/repository stack, the published SDK, a local HTTP
provider and SQLite. They are not a real shared-test team or PostgreSQL proof.
The temporary test changes were removed after preserving the exact patch and
results; application code and historical ownership remain unchanged.

`red-browser-entry.patch/json` records another RED on `1d6dd4f8`: opening the
real AuthSessionHydrator with `?workspace_id=team_alpha` performs only the
personal session/catalog reads, hydrates 13 personal storage keys, restores the
personal creation preference and opens its child workspace before team
authorization. This uses happy-dom, an axios adapter and in-memory localforage,
not a live browser or provider. Apply the patch from the repository root and
run its recorded command from `web/` after a frozen-lockfile install. The
first missing-dependency attempt is excluded. Existing session-cache,
session-reopen and realtime-client tests pass (9 outer tests; the nested
reopen harness is not counted again). The temporary harness was removed after
recording the patch. This identifies the context gate required before cache
hydration; it does not establish a new auth/session response contract.

- Record actual running Studio/Platform versions separately from this source
  baseline. Shared-test versions in historical notes are not current evidence.
- Continue failing host cases for old member generation and late selection
  responses, plus the full cache isolation matrix before implementing them.
- Pin a deep-linked object's true workspace; an unrelated Console preference
  must only affect new entries. A team default that Studio cannot serve must
  not display a personal canvas or use personal funds.
- Verify two distinct team members through the real provider and real product
  routes: different and conflicting edits, delete/undo interleaving, reconnect,
  creator exit, revocation during broadcasts, and retained original tasks.

ST0 is still in progress because those failing host cases and current runtime
inventory are outstanding. ST1–ST5 remain open. No production or shared-test
deployment, paid generation, real payment, email/SMS or customer-data mutation
was performed in this baseline pass.
