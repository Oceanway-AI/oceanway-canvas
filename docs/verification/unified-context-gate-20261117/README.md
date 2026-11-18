# Studio context gate — WP-STUDIO-018 ST1

This local candidate follows the failing baseline in
`../unified-context-baseline-20261117/`. It uses the published
`platformclient/v1.0.0-rc.5`, without a local replace.

The new `/api/auth/workspace-context` route revalidates Platform membership
before browser cache hydration. Stored personal canvas ownership wins over
navigation intent. Otherwise explicit intent wins over the confirmed session
selection; only an unset selection initializes personal context. Invalid,
unavailable or stale-member contexts never select personal funds.

Account reads use the SDK's `AccountSummaryForWorkspace`, with all intermediate
rotated cookies preserved. The browser pins the resulting page context instead
of using a subsequently changed Console preference. Login callback entry keeps
the original canvas ID. Late identity/context responses cannot hydrate another
actor's cache. Invalid and duplicate query parameters are rejected, including
the array syntax emitted by common serializers.

Team persistence is still absent: `supported=false` keeps team workspaces closed
with a retry action. This is an intermediate ST1 guard, not a complete team
feature or release candidate. ST2 ownership, real member revocation, shared rooms,
leases and collaboration must precede opening the team workspace. Accepted common
controls, ST3 media, ST4 purchase returns, Drama and joint acceptance remain open.

Reproduce the focused checks from the respective directories:

```sh
# backend/
GOWORK=off go test ./internal/handler \
  -run 'TestStudio(Workspace|TeamSummary|AccountSummary)' -count=1

# web/
bun test test/studio-workspace-entry.test.ts test/platform-session-cache.test.ts \
  test/platform-session-reopen.test.ts test/model-catalog-refresh.test.ts
```

The initial workspace tests failed on the prior source with HTTP 501; their
real route/SDK/SQLite fixtures now validate context resolution and rejection.
Browser-state cases run the real React hydrator/session/store in isolated
happy-dom processes with an axios adapter. These are free local fixtures, not
real-user browser, production PostgreSQL or deployed-provider acceptance.

No database schema, historical ownership or original task was changed. No shared
test/production deployment, model call, payment or real invitation was performed.
