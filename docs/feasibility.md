# Feasibility and evidence

## Initial integration choice

Keep Commonbeam's allocation and accounting independent of any proprietary
inference engine. The local reference path uses a provider daemon and a small
adapter interface. Existing-network adapters remain candidates rather than
assumed integrations.

| Candidate | Evaluation requirement before selection |
| --- | --- |
| Operator-owned vLLM or llama.cpp server | Approved model/license, declared content hash, bounded resources, actual streaming/usage tests, reproducibility profile and independent capacity checks. |
| Compute marketplace | Ability to operate the provider protocol, explicit lease/cost budget, independent router access and workable failure/identity boundaries. |
| Hosted inference network | Documented compatible requests, trustworthy usage evidence, privacy/retention terms, and no mandatory Commonbeam gateway. |
| Stake-based inference access | Capacity/epoch backing and portability; stake ownership must not become the sole public admission mechanism. |

The initial simulation avoids model licensing, GPU expenditure and third-party
account prerequisites while exercising real networking and PostgreSQL. It does
not decide which marketplace is cheapest or validate genuine AI service.

## Reproducible local evidence

The implementation was exercised on Windows amd64 with Go 1.27.3 and an isolated
loopback PostgreSQL 18.6 server, not an in-memory substitute.

Evidence includes:

- Two authority connection pools competing for bounded capacity, with budgets
  and holds conserved under concurrent requests.
- Idempotency under concurrent submission and across independently hosted
  router instances.
- Encrypted prompt/salt storage, distinct private commitments for identical
  jobs, idempotent commitment reuse, wrong-key rejection, process-level
  configuration persistence, cancellation acknowledgement and deadline recovery.
- Signed enrollment replay rejection, forged/tampered receipts, provider
  revocation, per-profile backing and unallocated pool reclaim boundaries.
- A compiled CLI's init/join/session/chat/balance/status journey.
- Restart of the compiled network without reseeding budgets, reuse of saved
  consumer sessions, and a separate CLI router with only its scoped credential.
- A native PostgreSQL dump/restore into a fresh isolated database with matching
  network identity, pool/account balances, jobs, consumption and journal counts.
- Python OpenAI client and native JavaScript client streaming against the local
  test network, with no external model API.
- Compatible HTTP fixtures covering streaming, missing usage, truncated output,
  changed models, excessive usage and unsafe backend targets.

Tests and native CLI checks are not evidence of real-model execution, GPU
capacity, an independent operator community, or production performance.
Container images/actions are immutably pinned and YAML is syntax checked;
container execution and hosted CI must be qualified separately.
Final source also cross-compiles for Linux amd64/arm64, Windows amd64 and macOS
arm64. Cross-compilation is not runtime qualification on those systems.
Windows race instrumentation could not run because no C compiler is available;
the Linux CI definition includes the race detector. No remote CI run is claimed.

## Capacity and funding model

For each exact model profile, measure:

```text
usable service capacity =
  measured sustained throughput
  * productive availability
  - verification and operational reserves

public capacity = funded, reserved share of usable capacity
average public allocation = public capacity / active demand

funding gap =
  measured hardware + power + network + operations costs
  - purchases - sponsorship - intentionally donated resources
```

Measure input processing, output generation, context/concurrency effects,
verification and failures independently. A throughput assumption is not a
benchmark. Network membership alone does not increase per-person capacity.

For an illustrative scenario only: 100 GPUs at 30 output tokens/second and 50%
productive utilization would yield 129.6 million output tokens per day. A 25%
public share among 10,000 users averages 3,240 output tokens per user per day.
At an assumed all-in $0.50 per GPU-hour, the resource cost is $1,200 per day.
These are arithmetic assumptions, not current prices, measured model performance
or a financial return forecast. Prefill/context and other excluded costs must
be incorporated before using a scenario to make commitments.

Local status reports completed jobs, consumed/held test units, account units
and unallocated pool balances. Test units and sponsor test balances do not cover
an actual electricity bill. No fixed economic conclusion follows from them.

## Explicit deciding factors

- Repository license and candidate model license/resource consent.
- Real CPU-model or GPU server availability and exact runtime/model revisions.
- Docker availability and supported native target environments.
- Measured verification mismatch rates and tolerable overhead.
- Public funding budget, actual operators and transport/security deployment.
- Governance and settlement assumptions, including whether a new coin is
  necessary at all.

Until these gates pass, retain the narrower claim: a functioning, persistent
local protocol/accounting demonstration with unqualified real-inference adapters.
