# ADR 0001: API/CLI-first test network

## Decision

Use Go, PostgreSQL, and a versioned HTTP protocol. A local network has one
logical accounting authority and independently operated routers/providers.
PostgreSQL transactions reserve both credits and provider capacity.

The initial backend is deterministic simulation. Qualified vLLM and llama.cpp
profiles attach through a compatible HTTP adapter; their model identity is
self-attested until independently checked. No model is downloaded automatically.

All funding and sponsorship are explicitly test balances. The initial local
network uses bounded public/contributor/paid pools, signed provider manifests,
provider receipts, and observation-only audits.

## Trust and privacy

The authority can administer test balances and enrollment. Authority failure
stops new admission. Routers cannot mint credits. Routers and executing providers
can see plaintext prompts. No financial slashing or confidentiality guarantee
is provided. A short-lived session is not a unique human.

The provider's outbound polling connection avoids opening inbound provider
ports. Only the local, test-only HTTP transport is qualified initially. Public
operation requires a separately approved transport/security deployment profile.

## Non-decisions

Repository/model licenses, public hosting, real payments, production token
issuance, decentralized settlement, and confidential inference require approval.
No custom blockchain, model engine, or hosted agent execution is introduced.

## Evidence requirements

Distinguish protocol tests, HTTP adapter fixtures, real PostgreSQL concurrency,
real-model execution, GPU execution, and genuinely independent operators.
Passing one does not satisfy the others.
