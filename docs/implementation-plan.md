# Implementation status and execution plan

The repository now implements a local test-network vertical slice. This is not
completion of the full network design. Continue with the next unblocked gate
and preserve the distinction between simulation, real inference, independent
operators, and production economics.

## Approved direction

API/CLI first for consumers, providers, sponsors, and independent routers. Use
Go and PostgreSQL, container definitions, deterministic simulation, and adapters
to existing serving engines. Browser chat is deferred.

The owner permits reversible technical decisions. Project/model licensing,
expenditure, public deployment, real funds, token launches, and security/trust
changes require approval. Initial operation is a disclosed single-authority
test network.

## Work-package status

| ID | Delivered | Remaining gate |
| --- | --- | --- |
| cb-decisions | Agent rules, trust/privacy ADR, scoped role boundaries | New consequential decisions still require owner approval. |
| cb-feasibility | Reference integration choice, resource model and explicit evidence classes | Real-model selection awaits license/resource approval. |
| cb-foundation | Go module, local CLI, PostgreSQL schema, fake backend, container and CI definitions | Container execution needs Docker; CI is configured, not claimed executed remotely. |
| cb-protocol | Versioned types, signed descriptors/manifests/receipts, API subset and rejection tests | Further profiles require separate conformance evidence. |
| cb-ledger | Transactional budgets and capacity, idempotency, expiry, cancellation and reconciliation | Production throughput and custody are not qualified. |
| cb-providers | Enrollment, key persistence, outbound work, health, drain, resume and revocation | Declared capacity is not proof of physical hardware. |
| cb-inference | Fake and compatible HTTP adapters; fixture-based streaming and failure tests | Real llama.cpp/vLLM model and GPU qualification blocked on approved resources. |
| cb-routing | Streaming, cancellation, independent routers, bounded reservations and explicit failures | Public transport/security profile is not approved. |
| cb-allocation | Public/contributor/test-paid pools, test sponsorship, finite grants and unallocated rebalancing | No per-human fairness guarantee or production subsidy model. |
| cb-auditing | Signed output checks, unpredictable post-completion selection and simulation replay | Real-model replay, meaningful adversarial detection rates and committee policy remain unqualified. |
| cb-onboarding | Init/join, role commands, diagnostics, examples and native test journeys | Container run, real hardware, cross-platform runtime, comprehensive diagnostics and release qualification remain open. |
| cb-economics | Aggregate test consumption and a transparent capacity/cost worksheet | No real-model cost/throughput or provider-retention evidence. |
| cb-release-readiness | Local test core, native backup/restore drill and documented validation procedures | Real backend, broader failure/load testing, deployment recovery qualification and licensing are required before release claims. |
| cb-pilot | No public service launched | Requires release gate, genuinely independent operators, hardware, funding and approval. |
| cb-independent-settlement | No new chain or custody introduced | Requires pilot evidence and explicit settlement/trust approval. |
| cb-web | Website deliberately not required | Optional static publicity/status, then browser chat after core readiness; hosting requires approval. |
| cb-agent-tools | API clients can keep credentials/tools local | A qualified bounded tool-calling agent example remains deferred. |
| cb-confidential | Ordinary plaintext exposure documented | Requires approved attestation/hardware/privacy model and compatible audits. |
| cb-token | No coins issued or issuance contracts created | Requires demonstrated necessity, economics, independent audits, legal review and approval. |

## Next executable decisions

The owner authorized pushing the current source and instructions on
November 2, 2026, while deferring further testing. Keep CI manual-only and do
not start tests or demo services until requested. Prior validation evidence
remains in `feasibility.md`; publication is not a new qualification result.

1. Owner selects a repository license and approves a candidate model's license
   and download/resource budget. Do not select or download one automatically.
2. Provide access to an approved local CPU model server or GPU server; run actual
   adapter conformance and reproducibility measurements. A compatible HTTP
   fixture is not this evidence.
3. Qualify container execution and supported native platforms, deployment
   recovery, hardware diagnostics and sustained overload behavior.
4. Measure service and verification costs, update pool headroom and epoch limits,
   and only then propose a funded, approved multi-operator pilot.

Independent work may improve local tests, documentation, metrics and packaging
while hardware/approval gates are blocked. It must not relax those gates or
rename simulations as successful production validation.

## Agent execution contract

Read `AGENTS.md`, the relevant ADR, protocol and code before a slice. Define the
specific acceptance test, implement the smallest complete change, run the
relevant checks, and record evidence/blockers here or in a focused decision
record. Do not invent an autonomous deployment/orchestration platform.

Keep assumptions and failed experiments visible. Commits, pushes, releases,
public hosting and hosted automation require the applicable execution authority;
do not infer them from an uncompleted work package.
