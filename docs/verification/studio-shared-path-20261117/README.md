# Studio shared path acceptance

Verification ran on 2026-11-17 Asia/Shanghai against the target `ow-test` at
`189.105.117.20`. Chromium used the real HTTPS hostname with an explicit host
mapping and normal certificate validation. The public entry is
`https://test.oceanway.tech/studio/`.

## Deployed source and artifacts

| Item | Value |
| --- | --- |
| Source | `f9b3c21e2d46c518f7a7f28a3d7eac239d813af5` |
| Runtime ancestor | `8274f1d0841760b3e66916d5921de53f04fc0831` |
| Integration base | `6534996` |
| Published SDK | `github.com/Oceanway-AI/oceanway-platform/platformclient v1.0.0-rc.2` |
| Web image | `oceanway-studio-path-web:f9b3c21e2d46` |
| Web image digest | `sha256:d432bdd25400bdd755779caed87e3f8c95dfac79c4594aaecd5b339a8b69563f` |
| Backend image | `oceanway-studio-path-backend:f9b3c21e2d46` |
| Backend image ID | `sha256:a9835ac224125097fd049ae99fbedfcb3889edb29a61579c762525d0449d87b6` |
| Backend binary SHA256 | `a1ada9bfb0dcfd2fe5a0e86cb7d257b568e8bdaf49cb320c53f4e3398cc6965a` |
| Web index SHA256 | `78b9d56cae77129c2fb0d1aab03674068315365f94a5504833f2d52718700b48` |

The build ran on `ow-test`, targeted linux/amd64 and used CGO for the backend,
matching its existing packaging. The runtime image preserves the migrated
backend image and replaces its executable. Both service containers passed
health checks with the source revision label above. The exact registry SDK was
used from its published module cache.

## Source verification

Frontend typecheck, touched-file ESLint and the production Vite build passed.
All 12 index asset references carry `/studio/` and exist in the built output.
Six targeted test files passed 19 tests in prefix mode with 114 assertions,
and 19 tests in root mode with 109 assertions. They cover path helpers, login
callbacks, auth entry, realtime URLs, material selection and upload recovery.
The backend `internal/platformintegration` tests passed.

The production frontend build retains existing chunk warnings. Three large
canvas files retain their pre-existing formatting warnings after unrelated
formatting changes were removed. Full modified-file formatting compliance is
not asserted.

## Browser results

`browser.json` records a fresh browser with empty storage opening the Studio
canvas route, displaying the Platform login iframe, completing a real login,
and receiving a Platform-backed Studio session. The standalone login return
also reaches `/studio/platform/login/complete`. Both `/studio/` and
`/studio/create` restore after a browser reload. Recorded static resource
responses contain no failed script, stylesheet or image requests.

`followup.json` records Console user 1 and Studio's matching local projection
in one browser context. A temporary empty canvas was created through the UI,
restored after deep-link reload, and opened twice over the public WebSocket
path with two `welcome` messages. Closing and reopening the page retained the
same canvas ID. Its final timeout came from waiting for the projects heading
at Drama's public landing route.

`cleanup.json` records the continuation to Drama's formal projects route,
where the same session returned `platform:1` from bootstrap and displayed
“我的项目”. It also records normal deletion of the temporary canvas and a
remaining canvas count of zero. Its final assertion documents the unexpected
SSE status detailed below. These two raw partial runs are retained alongside
their successful continuation.

`catalog.json` records an explicit wait for the complete model-catalog
response before navigating. It returned HTTP 200, code 0, source `platform`,
6,849,799 bytes and 70 channel models, including 65 available models, in
5.095 seconds. The model picker rendered a selectable item. Earlier backend
503 observations matched `context canceled` in Platform logs while the probe
navigated away before the catalog response completed. The completed request
and picker screenshots establish the loaded state.

## Remaining verification limits

`sse.json` records anonymous HTTP 401 and authenticated HTTP 500 for two absent
task IDs. No task existed for a live HTTP 200 event-stream check. The relevant
`text_replay.go`, `text_replay_cache.go`, `repository_tasks.go` and
`handler/response.go` files are byte-identical to runtime ancestor `8274f1d`.
`TaskForUser` returns GORM's missing-record error directly, and `failService`
maps this unclassified error to 500. This is source-level attribution to the
existing implementation. An independent old-runtime HTTP reproduction was
not performed.

The browser also observed the existing unimplemented announcement endpoints
returning 501, and the canvas initialization observed `/api/projects` returning
501. The tested canvas and login paths completed despite those responses.

Studio identity projection depends on the Platform API origin. Its ID is
UUIDv5 of the URL namespace and `origin + newline + Platform user ID`, and its
identity subject is the SHA256 hex digest of the same string. Switching the
API origin to the root hostname produced a new local projection for the same
Platform account. The parent migration task subsequently normalized the four
retained projections and merged the duplicate. Its database receipt owns the
row-count verification.

`final.json` and `final.cjs` record the browser recheck after that reconciliation
and backend restart. The existing session reached Console, Studio and Drama
as Platform user 1, the Studio session used the expected root-origin UUID,
and login preparation retained the prefixed callback. Runtime health returned
the deployed source revision. The complete catalog again returned 200 with
70 models and 65 available models, in 5.236 seconds. The model picker rendered
and the temporary canvas remained absent.

## Captures

`login-frame.png` was captured before credential entry.
`canvas-reopened.png` shows the restored temporary empty canvas.
`same-session-drama.png` shows the authenticated Drama projects page.
`create.png` preserves the earlier state before the model catalog finished.
`create-catalog-loaded.png` and `model-picker-loaded.png` show the fully loaded
model catalog state. `final-studio-models.png` shows the loaded picker after
identity reconciliation. Session state and credentials remain private outside
this repository.
