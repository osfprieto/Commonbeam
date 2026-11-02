# Local test protocol

The reference wire types are `internal\protocol\protocol.go`. The consumer
subset is described in `api\openapi.yaml`. Use protocol version `commonbeam/v1`.
Unknown JSON request fields, extra JSON values, non-text messages and unsupported
roles are rejected. Request bodies are capped at 64 KiB.
Browser-origin requests are disabled in this API/CLI-only pilot to prevent
drive-by consumption of loopback test pools. An approved browser application
will need an explicit origin/security profile rather than permissive CORS.

## Trust and access

The descriptor's public key must match an independently obtained fingerprint
before joining. Descriptor signatures bind the network ID, endpoints and policy.
A joined consumer receives no enrollment, funding, router, or administrator
privileges. Operators receive only their needed out-of-band scoped credential.
The CLI can read those credentials from environment variables instead of an
administrator's `secrets.json`.

All authority calls use scoped bearer credentials. Routers pass the consumer's
session in `X-Commonbeam-Account` to authority operations; a router credential
alone is insufficient to debit an unknown account. Never expose the pilot's
HTTP-only transport publicly.
The selected router does receive the consumer bearer credential and is trusted
with that finite test allowance as well as plaintext. It is not a cryptographic
per-request spending sandbox. Stronger request capabilities are a prerequisite
to reconsidering this boundary for real funds or untrusted routers.

The network admits a single exact model/hash/profile and one test epoch.
Additional providers must cover that epoch. Model variants and privacy classes
are not silently substituted. New epochs currently require an explicitly new
test network/database; no old allocation carries a perpetual promise.

## Authority endpoints

Authority defaults to loopback port 8081; routers default to loopback port 8080.

| Method and path | Credential | Payload / result |
| --- | --- | --- |
| GET `/healthz` | None | Database readiness; no configuration details. |
| GET `/commonbeam/v1/status` | None | Aggregate test balances and trust labels. |
| GET `/commonbeam/v1/providers` | None | Public signed manifests and availability; no tokens or backend URLs. |
| POST `/commonbeam/v1/enroll` | Enrollment | Signed manifest; one-time provider token. Replay does not rotate it. |
| POST `/commonbeam/v1/heartbeat` | Provider | Refresh liveness without replenishing capacity. |
| POST `/commonbeam/v1/resume` | Provider | Explicitly reactivate an unexpired provider. |
| POST `/commonbeam/v1/drain` | Provider | Stop new work without revoking accepted jobs. |
| GET `/commonbeam/v1/work` | Provider | Claim only this provider's reserved work. |
| POST `/commonbeam/v1/work/{id}/append` | Provider | `{"text":"..."}`; at most 4 KiB per chunk and 32 KiB per result. |
| POST `/commonbeam/v1/work/{id}/finish` | Provider | `{"receipt":<signed receipt>}`; null receipt reports failure. |
| POST `/commonbeam/v1/contribute` | Provider | One test allowance per enrolled provider, drawn from the contributor pool; not hardware proof. |
| POST `/commonbeam/v1/fund` | Sponsor | `{"pool":"public","units":4096}`; requires unallocated declared test capacity. |
| POST `/commonbeam/v1/reallocate` | Sponsor | Transfer only unallocated balances between pools; existing grants/reservations cannot be reclaimed. |
| POST `/commonbeam/v1/revoke/{id}` | Administrator | Invalidate provider credentials; accepted jobs retain bounded reconciliation. |
| POST `/commonbeam/v1/grant` | Administrator | Transfer a bounded pool allowance into an account; does not create funding. |
| POST `/commonbeam/v1/session` | Router | Transfer 4096 units from the public pool into a pseudonymous account. |
| GET `/commonbeam/v1/balance` | Router + account | Available account balance; expired epochs cannot be redeemed. |
| POST `/commonbeam/v1/jobs` | Router + account | `{"request":<chat>,"idempotency":"..."}`; atomic credit/capacity reservation. |
| GET `/commonbeam/v1/jobs/{id}` | Router + account | Own state, cumulative output, usage and receipt; no prompt. |
| POST `/commonbeam/v1/jobs/{id}/cancel` | Router + account | Request cancellation; running capacity stays held until acknowledgement or expiry. |
| GET `/commonbeam/v1/audit/{id}` | Administrator | Observation-only signature/output checks and simulation replay; no financial penalty. |

Routers expose model discovery, chat, session creation, balance, own job state,
status and health through their own endpoints. Use the same session and
idempotency key across routers to replay a result rather than creating new work.

## Signed payloads

Ed25519 signatures cover a UTF-8 domain line followed by the compact JSON
serialization of the typed payload, not the enclosing signed object.
The reference encoder uses Go `encoding/json`, struct field declaration order,
no insignificant whitespace, and its standard JSON string escaping. Integrators
must reproduce those exact bytes; arbitrary JSON key reordering is not canonical.

Domains are `commonbeam:manifest:v1`, `commonbeam:receipt:v1`, and
`commonbeam:descriptor:v1`, each followed by a newline. Public keys and signatures
use standard padded base64. Provider IDs are the first 32 lowercase hexadecimal
characters of SHA-256 over the base64 public-key string.

A manifest's field order is version, network, provider, model, model_hash,
profile, backend, capacity, slots, expires, public_key. Receipt order and nested
usage fields are fixed by the versioned Go type. The test suite checks tampering,
cross-network binding and deterministic serialization.

Request hashes normalize `stream` to false before compact encoding; choosing
streaming rather than buffered transport does not authorize duplicate execution.
An idempotency key reused for different semantic input returns a conflict.
Validation normalizes absent temperature to explicit zero before hashing and
forwarding; runtime defaults must not change the declared test profile.

New jobs use a random 192-bit private salt, encoded as 48 hexadecimal characters.
It is encrypted together with the request and supplied only through authorized
job access, not in public receipts or aggregate status. The request commitment
is SHA-256 of `commonbeam:request:v1`, newline, salt, newline, and the hexadecimal
semantic request hash. The output commitment is SHA-256 of
`commonbeam:output:v1`, newline, salt, newline, and the UTF-8 output.
This prevents stored commitments becoming an unsalted dictionary of private
questions/answers when evidence keys are unavailable. Early local demonstration
records remain readable with their legacy unsalted format; privacy is not
retrofitted into old receipts.

## Metering and outcomes

Admission reserves a conservative input byte budget with message overhead plus
maximum output units. This is not a universal model tokenizer. Upstream usage
must be present, internally consistent, and within the reservation; otherwise
the job fails rather than charging beyond authorization.
The router conservatively reports `finish_reason=length` when reported output
usage reaches the authorized maximum; it does not claim full upstream metadata
compatibility.

Accounts support one in-flight request. Providers support only declared bounded
slots. Accounting writes are deliberately serialized by a PostgreSQL
transaction-scoped advisory lock across authority instances; this is a
correctness-first pilot, not a high-throughput settlement design.

Before acceptance, cancellation releases the reservation. After acceptance,
the slot and credits remain held until the provider acknowledges cancellation
or the fixed lease expires. Pilot cancellation/failure refunds the reservation;
the operator bears any work already consumed. No real-money policy is implied.
The provider must obey the job deadline. A lost completion is not re-executed.

Prompts and output are encrypted at rest using the network evidence key.
Terminal evidence is logically cleared after 24 hours; receipt commitments and
accounting remain. This is not secure erasure of WAL, storage snapshots or
backups. Retain the same evidence key for database recovery.

After evidence expiry, result replay is refused and audits report expired
evidence. The challenge window and ordinary retention are the same in this
local prototype. Routers/providers still see plaintext while executing.
