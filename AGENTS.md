# Commonbeam development rules

Implement bounded, tested vertical slices. Read `docs/implementation-plan.md`
and the relevant decision records before changing behavior.

- This is a local test network, not production currency or verified AI.
- Public access must not require buying coins or contributing hardware.
- PostgreSQL is authoritative. Never replace persistence/concurrency tests with
  an in-memory ledger and call the distributed behavior validated.
- Simulation, adapter conformance, real-model execution, and independent
  operators are separate evidence classes.
- Keep inference off-chain. Receipts and declared model hashes are not proofs
  of execution, factual accuracy, or genuine demand.
- No paid services, public deployments, real funds, token issuance, license
  selection, or changes to trust/privacy assumptions without owner approval.
- Do not download models without license and resource consent.
- Bind to loopback by default. Never log prompts, credentials, or database URLs.
- Preserve budget/capacity conservation, idempotency, and explicit failure states.
- Do not silently downgrade a model, privacy class, or verification profile.
- For code changes, run `go fmt`, `go vet`, and targeted tests unless the owner
  has explicitly deferred execution. Real database tests require
  `COMMONBEAM_TEST_DATABASE_URL`; report skipped hardware tests honestly.
- Record evidence and blockers in the roadmap. Do not create an agent
  orchestrator, automatic token launcher, or unattended deployment system.
- No repository license has been selected; do not imply redistribution rights.

## Current handoff

The local test-network core is implemented; the full roadmap is not complete.
Use `docs/implementation-plan.md` for remaining gates and `docs/feasibility.md`
for prior evidence. Do not treat earlier results as a fresh validation run.

The owner authorized publishing the implementation and instructions on
November 2, 2026, but deferred further testing. Do not launch test/demo services,
run validation suites, or dispatch CI until testing is requested again. CI is
manual-only (`workflow_dispatch`); do not restore automatic push/PR triggers
while this deferral is active.

Resume with repository/model licensing and approved real-model resources, then
real-runtime, container, and independent-operator qualification. Publishing
source does not authorize deployments, spending, model downloads, or tokens.
