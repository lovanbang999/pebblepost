# Request Scripts

Requests may define `scripts.preRequest` and `scripts.postResponse`; folders may define the same fields in `_folder.pebble.json`. Pre-request scripts run from the workspace root toward the request. Post-response scripts run for the request first, then from the nearest folder back toward the root.

The Rust server executes scripts in a JavaScript runtime with a bounded memory budget, stack size, and timeout. The `pb` API (also exposed as `pm` for Postman compatibility) supports request URL, method, headers, and raw body changes; environment and local variables; runner flow control; response inspection and assertions; console logs; crypto, date, random, and base64 helpers; and synchronous `pb.sendRequest`. Auxiliary requests are limited to five per script and use the script timeout. `scriptTimeoutMs` in request settings overrides the five-second default.

Scripts are skipped when the request execution marks its workspace untrusted. Pre-request script errors stop the request; post-response assertion failures are returned in the response `tests` list. `pb.sendRequest` rejects cloud metadata hosts and limits response bodies. Do not put secrets in script source; secret values are masked in script logs and auxiliary request errors.

The persistent WebSocket/SSE connection API does not run request scripts. The Rust server now has update check, settings, signature verification, and apply endpoints; updater behavior still needs platform comparison. Several CLI automation commands remain migration gaps, and OS keychain support needs platform comparison. Keep Go sources until the remaining behaviors have Rust parity.
