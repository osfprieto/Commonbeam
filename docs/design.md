# Proposed design

This document records the initial proposal, not implemented behavior or a
production security guarantee. Changes should state their assumptions and
include evidence from the milestones in the [implementation plan](implementation-plan.md).

## 1. Design principles

1. Public access must not require buying cryptocurrency or contributing hardware.
2. Scarcity and funding limits must be visible rather than disguised as
   unlimited service.
3. Clients, routers, and providers must be independently replaceable.
4. Resource commitments must not be allocated or sold twice.
5. Rewards must track independently checked service capacity, not self-reported
   hardware or easily manufactured demand.
6. Privacy, verification, and governance trust assumptions must be explicit.
7. Prefer existing infrastructure until a measured requirement justifies new
   infrastructure.

## 2. System boundaries

```text
User application or local agent
              |
      User-selected router
              |
    Admission and reservation
              |
    Independent model provider
              |
       Streamed response

Separate control and accounting path:
registry + capacity commitments + signed receipts
              |
   allocation / audits / disputes
              |
        ledger settlement
```

### Components

| Component | Responsibility |
| --- | --- |
| Client | Select routers, submit requests, display allocation and privacy information, and manage local credentials. |
| Provider registry | Publish signed service manifests, endpoints, capacity commitments, and eligibility status. |
| Router | Discover eligible providers and route admitted requests; users can self-host or bypass it. |
| Admission and allocation | Reserve capacity, enforce pool budgets, and prevent concurrent redemption of the same credits. |
| Provider | Execute an approved model/runtime profile and produce streamed results and signed receipts. |
| Auditor | Independently test capacity and re-execute selected requests within a declared verification profile. |
| Settlement | Record eligible rewards, fees, deposits, challenges, and dispute decisions. |

Registry and accounting state must be sufficient for an independent client or
router to operate. The project's website must not be the only endpoint able to
issue, redeem, or inspect an allocation.

Evaluate adapters to existing compute networks first. An adapter must document
which properties Commonbeam controls and which depend on the upstream network.
Supporting multiple upstream networks is not automatically equivalent to
portable credits or decentralized settlement.

## 3. Execution model

Begin with one small open-weight model, approximately 8B parameters with a
suitable quantization, on a narrowly defined hardware/runtime profile. Actual
memory, concurrency, context, latency, and throughput limits require measurement.
Model licensing must allow the intended hosting and distribution.

The default provider serves complete requests from a model replica. Larger
models may later use local multi-GPU clusters. Splitting each response across
unrelated internet-connected households is not an MVP requirement.

A signed provider manifest identifies:

- Model, tokenizer, quantization, and runtime content hashes.
- Hardware and verification profile.
- Context, concurrency, and availability limits.
- Capacity committed for a specific epoch and published resource prices.
- Supported privacy mode, retention policy, and provider restrictions.

Increasing provider count should primarily increase independent request
capacity. Do not promise that it increases model quality or individual response
speed.

## 4. Request lifecycle

1. The client chooses a model profile, privacy class, and maximum resource budget.
2. A router discovers providers and requests an expiring capacity reservation.
3. Admission atomically reserves credits and provider capacity before execution.
   The initial implementation may use a shared service, but must disclose that
   dependency and its failure mode.
4. The provider accepts a request identifier and streams the response. Retries
   use idempotency identifiers to avoid duplicate charges.
5. The provider signs a receipt binding the request commitment, execution profile,
   usage, result commitment, and reservation identifier.
6. Unused reservations are released; failures, cancellations, and partial streams
   follow published charging and refund rules.
7. Audit selection occurs after the result commitment. Rewards remain unsettled
   for a defined challenge window.

Receipts are accounting evidence, not proof that a human wanted the result or
that the answer is factually correct. Disputes require available execution
evidence; the design must address refusal to provide that evidence.

## 5. Capacity and access

### Accounting instruments

**Service credits** represent expiring access to reserved capacity for a defined
model class and epoch. Begin with nontransferable credits. Credit conversion
between model classes requires admission at published rates and available
capacity, not an unconditional exchange promise.

A potential **network coin** serves payments, deposits, and provider rewards.
Initial experiments use test assets. A production coin is conditional on
evidence that existing settlement assets cannot satisfy the coordination need.
Coin ownership must not be a prerequisite for public access.

Meter input processing, output generation, and context-related resource use
separately. Do not assume one request or one text token has a constant cost across
models and workloads.

### Allocation pools

The following split is an experiment, not a guaranteed entitlement:

| Pool | Illustrative share of net usable capacity | Admission |
| --- | --- | --- |
| Public commons | 25% | Sponsored, bounded access without a purchase. |
| Contributors | 55% | Credits earned through verified contribution. |
| Paid overflow | 20% | Usage-based purchases; an explicit portion supports public service. |

Calculate net usable capacity after headroom, audits, reliability reserves, and
operating overhead. Track each reservation once. Unused capacity may be borrowed
under explicit reclaim and priority rules.

The split must change if funding or measured capacity cannot support it. Paid
overflow does not automatically fund the other pools. Sponsors and pilot
subsidies require explicit budgets and exhaustion behavior.

### Fairness and abuse

Do not grant an allowance to every wallet: wallets are not unique people.

Offer a globally capped anonymous best-effort queue without a guaranteed
per-person allowance. Evaluate optional privacy-preserving credentials from
multiple independent communities for stronger sponsored allocations. Credentials
introduce issuer and identity assumptions and must not become one mandatory
corporate account.

Bound output, context, concurrent jobs, and agent execution. Define protections
against queue flooding, repeated expensive admissions, and reservation hoarding.
Publish queue and allocation behavior under overload.

## 6. Incentives and sustainability

An illustrative scoring rule is:

```text
score = audited reserved capacity
        * availability
        * successful delivery factor

epoch issuance reward = fixed epoch budget
                        * eligible provider's fraction of total scores
```

This is a hypothesis to test, not a finished incentive mechanism. Define scoring
windows, normalization, zero-score behavior, audit coverage, and budget limits
before implementing rewards.

Challenge capacity through unpredictable, concurrent work rather than reported
GPU specifications. Prevent the same capacity being pledged to overlapping
reservations. Avoid issuance rewards proportional to request counts or output
length: providers and clients can manufacture both.

Separate fee income from issuance rewards. If a coin is justified, enforce a
published issuance schedule and hard epoch ceilings. Do not select a token
supply, price target, or yield promise before modeling the economics.

Report actual resource costs, utilization, fee and sponsorship income, public
subsidies, issuance, and provider retention. Model outages, demand shocks, and a
severe decline in subsidy value. Minting does not pay an electricity bill unless
someone accepts the resulting asset.

## 7. Verification and disputes

Start with a restricted reproducible execution profile. Pin weights, tokenizer,
runtime, hardware class, decoding, and randomness. Temperature zero alone is
not a sufficient reproducibility guarantee.

Use signed commitments followed by unpredictable audit selection, independent
re-execution, delayed settlement, and provider exposure limits. Calibrate audit
rates against measured attack detection, costs, and false positives.

For a beta, evaluate randomly selected bonded verifier committees, for example
three agreeing members out of five. Model committee capture, correlated
operators, false accusations, unavailable evidence, and appeals.

Such a committee is an explicit honest-quorum trust assumption, not a trustless
proof. A contract cannot resolve conflicting LLM outputs merely by receiving
both outputs. Sampling and reputation are also not universal correctness proofs.

Do not automatically slash subjective answer quality, factual mistakes, or
normal numerical variation. Punitive decisions require clearly defined evidence
within the approved execution profile. Start in observation-only mode before
enabling financial penalties.

Cryptographic proofs and confidential execution are candidate upgrades, not
assumed solutions. Benchmark their end-to-end costs and threat models before
making security claims.

## 8. Privacy and data availability

Ordinary remote execution exposes plaintext to the executing provider.
Transport encryption does not remove that trust.

A later confidential class may use attested trusted execution environments.
Document hardware, firmware, key-release, and attestation trust assumptions.
Auditors must not silently bypass the selected privacy class.

Keep prompts and responses off-chain. Use salted commitments and encrypted,
time-limited audit records. Choose challenge windows, evidence access, deletion
rules, and refusal penalties together. Do not promise immediate deletion and
later full re-execution of the same private request.

Minimize request metadata in public receipts. State what routers, providers,
auditors, credential issuers, and settlement observers can learn.

## 9. Agents, governance, and exit

Run agent controllers locally by default, keeping tool credentials and
permissions on the user's machine. Hosted agents require isolation, step and
spend limits, revocation, and approval for consequential external actions.
Meter tool execution separately from inference.

Use public proposals, delayed upgrades, documented emergency powers, and
independently usable software. Do not assume coin voting alone provides fair
governance. Users must be able to inspect policies, choose supported model
versions, and move to independent routers.

The first implementation may have centralized components. Every such dependency
must have an owner, documented failure behavior, and an explicit acceptance or
replacement decision before broader release.

## 10. Open decisions

- Existing-network integration versus operating Commonbeam providers.
- Model, runtime, hardware profile, and implementation languages.
- Credit accounting, concurrency control, and settlement infrastructure.
- Pricing and cost normalization across model classes.
- Audit sampling, evidence availability, committee selection, and appeals.
- Public admission and optional credential mechanisms.
- Privacy classes, retention windows, and confidential execution.
- Governance, repository license, and any production token mechanism.

Resolve these with experiments and recorded decisions rather than treating this
initial proposal as a finalized protocol.
