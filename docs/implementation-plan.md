# Implementation plan

This is a gated delivery plan, not a launch promise. No phase is implemented yet.
Later phases depend on evidence from earlier ones. Do not issue a production
token or promise a public allowance before service, security, and funding are
validated.

The [design](design.md) defines the initial architecture and its open decisions.

## Phase 0: scope, threat model, and feasibility

**Deliverables**

- Select a repository license and review candidate model/runtime licenses.
- Compare integrating existing compute networks with operating a small provider
  network; record cost, control, privacy, and settlement constraints.
- Select one model, hardware class, and runtime for a reproducibility spike.
- Define actors, trust boundaries, adversaries, and initial centralized services.
- Build a capacity-and-cost model covering input, output, context, utilization,
  auditing, idle capacity, failures, and public-pool funding.
- Define workload fixtures and measurable latency, availability, cost, fairness,
  and verification targets before evaluating results.
- Record implementation-language and infrastructure choices with alternatives
  and reasons.

**Exit gate:** publish a measured feasibility report and threat model. Proceed
only with a plausible funding model and an execution profile whose verification
limits are understood. Otherwise narrow scope or prefer an existing network.

## Phase 1: a replaceable inference path

**Deliverables**

- A minimal client, self-hostable router, and provider interface.
- Signed provider manifests and model/runtime identity checks.
- Streaming, context/output limits, cancellation, timeouts, retries, and
  idempotent request identifiers.
- Reproducible local development instructions and a minimal operator guide.
- Structured measurements without unnecessary prompt logging.

Use a small deployment with at least two independently operated providers.
Accept an explicitly documented centralized registry for this phase if needed.
Do not introduce a blockchain merely to route inference.

**Exit gate:** an independent client can discover a provider, switch routers,
complete a request, and observe a provider failure without duplicate charging.
Confirm experimentally which discovery or admission operations still depend on
project-operated services.

## Phase 2: contribution accounting and bounded access

**Deliverables**

- Epoch capacity commitments, expiring model-specific credits, and atomic
  reservation/redemption.
- Public, contributor, and paid-pool accounting using test payments.
- Signed usage receipts and documented partial-response/refund behavior.
- Capacity headroom, borrowing/reclaim rules, and budget-exhaustion behavior.
- A capped anonymous queue and an initial abuse-control implementation.
- Funding and allocation dashboards that distinguish real income from subsidies.

Keep allocation services and policy versions auditable. Any central admission
service remains a disclosed dependency, not a decentralization claim.

**Exit gate:** concurrency tests show no double redemption or overlapping
allocation of the same capacity. Cancellation releases the correct reservation.
Overload and exhausted funding produce bounded, documented behavior rather than
unfunded credit issuance.

## Phase 3: audits and adversarial accounting

**Deliverables**

- Unpredictable capacity challenges and post-commitment audit selection.
- Re-execution measurements on the approved runtime/hardware profile.
- Evidence retention, challenge deadlines, and refusal handling.
- Observation-only reward scoring and a dispute simulator.
- Tests for cheaper-model substitution, false capacity claims, client/provider
  collusion, replayed receipts, withheld evidence, and dishonest auditors.

Measure false positives and false negatives. Treat reliable transport and
correct accounting separately from proof of declared model execution.

**Exit gate:** publish attack results, detection assumptions, audit costs, and
honest-execution mismatch rates. Define acceptable thresholds before enabling
financial penalties. If verification is inadequate, retain explicit trusted
provider or committee assumptions rather than claiming trustlessness.

## Phase 4: controlled multi-operator pilot

**Deliverables**

- A pilot sized to available resources, initially targeting 10-20 GPUs across
  independent operators rather than a fixed promised capacity.
- A sponsored public pool with a stated budget, duration, and demand limits.
- Contribution-earned access and optional paid overflow.
- Provider failure drills, queue-flooding tests, and reservation-hoarding tests.
- Independent-router operation, provider replacement, and outage recovery.
- Workload and accounting reports using privacy-preserving telemetry.

The illustrative 25/55/20 allocation is a scenario to test, not a required
configuration. Pilot users receive no perpetual service entitlement.

**Exit gate:** users without cryptocurrency or hardware can obtain bounded
service; contributors can redeem earned credits with another participating
provider; independent operators can reproduce deployment; costs stay within the
published pilot budget.

## Phase 5: settlement and decentralization hardening

**Deliverables**

- Decide whether an existing settlement asset is sufficient.
- Prototype deposits, delayed payouts, epoch reward ceilings, disputes, and
  appeals with test assets on an existing chain if justified.
- Remove or explicitly accept each centralized registry, router, admission,
  indexing, and key-management dependency.
- Test independent clients against project gateway and frontend outages.
- Document chain, RPC, committee, upgrade, and administrative trust assumptions.
- Obtain independent review before putting real funds at risk.

**Exit gate:** independent operators can discover, admit, execute, and settle
eligible requests without an indispensable project-operated gateway. The
remaining trust assumptions and failure/recovery procedures are public.

## Phase 6: optional capabilities and production decision

Evaluate each item independently; none is a prerequisite for demonstrating the
core public-access model:

- Additional models and hardware classes with separate verification profiles.
- Multiple upstream-network adapters and explicit cross-network settlement.
- Optional community credentials and privacy-preserving allocations.
- Attested confidential inference with compatible audit procedures.
- Local agent tooling, followed by isolated hosted agents if justified.
- More advanced proofs, only after representative end-to-end benchmarks.
- A production network coin, only if existing assets demonstrably fail to meet
  a documented coordination need.

**Production gate:** complete security and operational review, funding stress
tests, governance and upgrade procedures, and jurisdiction-specific legal review
where applicable. Do not launch merely because token contracts can be deployed.

## Required validation

Select tooling after the implementation stack is chosen. Add tests alongside
the behavior each phase introduces, including:

| Area | Evidence required |
| --- | --- |
| Inference | Reproducible runtime profile, streaming/cancellation behavior, measured latency and throughput under realistic contexts. |
| Accounting | Conservation of pool budgets, single redemption under concurrency, idempotent receipts, and correct release/refunds. |
| Verification | Deliberate substitution and capacity attacks, audit unpredictability, mismatch rates, and evidence-availability behavior. |
| Fair access | Bounded anonymous demand, overload behavior, reservation abuse, and public-budget exhaustion. |
| Privacy | Data-flow and retention review, no prompts on-chain, and no auditor bypass of the declared privacy class. |
| Economics | Measured operating cost, funding coverage, provider retention, and sensitivity to utilization and falling subsidies. |
| Independence | A separately operated client, router, and provider remain usable when the main gateway or frontend is unavailable. |

Use integration and load tests for distributed behavior, not only unit tests or
happy-path demos. Publish assumptions and failed experiments as well as successes.

## Planning assumptions

A narrow prototype may take approximately 3-4 months with 4-6 experienced
engineers; an adversarial multi-operator beta may take 6-12 months in total.
These are preliminary estimates, not staffing commitments. Available compute,
integration constraints, reproducibility, and security findings may change them.

The next concrete work is **Phase 0**, not a token launch or production
infrastructure deployment.
