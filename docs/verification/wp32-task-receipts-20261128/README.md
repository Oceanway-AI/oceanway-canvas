# WP32 Task Receipt Adoption

2026-11-28. Shared test deployment preparation based on the running Studio revision `cdafaa741af4349b6699d6aa731b967961eae12a`.

Studio adopts the published PlatformClient `v1.0.0-rc.8`, pinned to provider commit `f0fa62f5144fe86b9922a88d7b515f8041a85353`. Module checksum is `h1:Vpj4+p5wjppFzobZwD51LNCbkDsDy9RiF/woGwvV9as=`.

Ordinary and Agent media generation commands declare `task-funding.v1`. Native request bodies and the original authorization remain intact. Pending native receipts may omit `funds_state`; Studio retains the original task and follows the server's retry interval. Existing final settlement and saved-output checks remain authoritative.

Validation used the released module with `GOWORK=off`. The original-admission regression first failed for the missing contract declaration. Five handler tests then passed against an owned PostgreSQL cluster with isolated Redis, covering submission, receipt recovery after lost acknowledgement, early outputs, synchronous completion and browser adoption. The Agent command test, relevant package vet and backend build passed. Temporary test dependencies were stopped.

The local `.local/wp32/go.work` was used only during the first implementation check. Final verification used the actual rc.8 dependency in `backend/go.mod` and its recorded module checksum.
