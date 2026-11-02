# Commonbeam

**Shared compute. Open intelligence.**

Commonbeam is an API/CLI-first project for community-powered AI inference,
contribution-earned access, and a sustainable public compute commons.

**Current implementation: local test network.** The working demonstration uses
simulated inference and test balances. It is not a public decentralized service,
a cryptocurrency, proof of model execution, or a promise of free AI capacity.

## Goal

Independent operators contribute compute and earn access or rewards. A
transparently funded share of usable capacity provides bounded public access
without requiring a subscription, cryptocurrency purchase, or hardware.

Commonbeam follows the DePIN pattern on the supply side. Its intended focus is
the **commons allocation layer**: public access without a capital prerequisite,
contribution-earned service, portable provider/router choice, visible scarcity,
and funding that can survive declining token subsidies. These commitments are
not claims of novelty or capabilities already demonstrated at production scale.

## What works locally

- Go CLI, authority, independent routers, and outbound-polling provider workers.
- PostgreSQL reservations, finite pools, idempotency, cancellation, recovery,
  signed receipts, and encrypted temporary request/result evidence.
- Bounded public sessions, contributor allowances, test sponsorship, test-paid
  grants, and unallocated pool rebalancing.
- Streaming text chat and model discovery through a documented compatible API
  subset, with Python and JavaScript client examples.
- Deterministic simulation and HTTP adapters for separately qualified vLLM and
  llama.cpp servers. No model is downloaded automatically.
- Observation-only audits, provider draining/revocation, diagnostics, container
  deployment definitions, and CI configuration.

There is **one accounting authority and one exact model/profile per test
network**. Real-model/GPU qualification, genuinely independent operators,
production economics, and public operations remain separate gates.

## Try the local network

Requires Go 1.27.3 and a dedicated local PostgreSQL database. The example uses
PowerShell; set the URL using your own local database credentials.

```powershell
go build -o bin\commonbeam.exe .\cmd\commonbeam
$env:COMMONBEAM_DATABASE_URL = "<local PostgreSQL connection URL>"
.\bin\commonbeam.exe dev
```

In another terminal:

```powershell
.\bin\commonbeam.exe session
.\bin\commonbeam.exe models
.\bin\commonbeam.exe chat --prompt "Hello, Commonbeam"
.\bin\commonbeam.exe balance
.\bin\commonbeam.exe status
```

The answer explicitly says that no model was executed. `dev` starts two
simulated providers and seeds finite test budgets once. It does not replenish
funding automatically. Keep `.commonbeam` private and retain it across restarts.

For containers, independent consumers/operators, real-backend attachment, and
recovery, see the [operator guide](docs/operators.md). Docker definitions are
provided, but Docker execution must be qualified on an equipped host.

## Development

Further testing is currently deferred at the owner's request. CI is
manual-only (`workflow_dispatch`), so pushes and pull requests do not start
test runs automatically. The commands below are for when testing resumes.

```powershell
go fmt ./...
go vet ./...
$env:COMMONBEAM_TEST_DATABASE_URL = "<dedicated test PostgreSQL URL>"
go test ./... -count=1 -timeout=90s
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

Database tests create and remove their own uniquely named schemas; use a
dedicated test database with schema-creation permission, never production.
Without the test URL, database/integration tests explicitly skip and do not
establish persistent or concurrent correctness.

## Design and evidence

- [Architecture proposal](docs/design.md)
- [Implementation status and remaining gates](docs/implementation-plan.md)
- [Protocol and trust boundaries](docs/protocol.md)
- [Operator and client journeys](docs/operators.md)
- [Feasibility and validation evidence](docs/feasibility.md)
- [API subset](api/openapi.yaml)
- [Development rules](AGENTS.md)

A publicity website is optional. No website is required for discovery,
allocations, the CLI, or independent routers.

## Safety and license status

All funding is test-only. Public sessions are not unique-person identities.
Routers and providers see plaintext prompts. At-rest encryption does not provide
confidential inference. Receipts attest to signatures, not honest execution.
Database owners and the accounting authority remain trusted.

Do not expose the HTTP-only pilot to the internet, handle real funds, launch a
token, or claim trustless service. Licensing, model downloads, paid services,
public deployments, and changes to trust/privacy assumptions need owner approval.

**No repository license has been selected.** Public visibility does not grant
redistribution rights; a license decision is required before external
distribution or contributions.
