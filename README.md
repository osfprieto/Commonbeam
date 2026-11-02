# Commonbeam

**Shared compute. Open intelligence.**

Commonbeam is a proposed open protocol for community-powered AI inference,
contribution-earned access, and a sustainable public compute commons.

**Status: design and implementation planning.** There is no running network,
implemented protocol, issued token, or guaranteed inference allowance.

## Goal

Build a community-governed network where independent operators contribute
compute and earn rewards or service credits. Allocate a transparently funded
share of usable capacity to public access without requiring a subscription,
cryptocurrency purchase, or hardware contribution.

Users should be able to choose providers, run their own clients and routers, and
understand how access is allocated when demand exceeds supply. Verification
assumptions, operating costs, and funding coverage must be explicit.

## What makes Commonbeam worth building?

Commonbeam fits the decentralized physical infrastructure network (DePIN)
pattern: independent hardware operators contribute resources and receive
incentives. Its proposed focus is the **commons allocation layer**, not merely
another GPU marketplace or cryptocurrency.

The commitments to test are:

- **Public access without a capital prerequisite:** a bounded, funded pool for
  people who do not own GPUs or cryptocurrency.
- **Contribution-earned access:** verified resource contributions earn access to
  participating providers.
- **Portable access:** changing a participating provider or router should not
  require abandoning an allocation or creating a new platform account.
- **Transparent scarcity:** published admission, queueing, and allocation rules
  instead of a promise of unlimited service.
- **Sustainable funding:** distinguish purchases, sponsorship, donated compute,
  and token subsidies, and test viability as subsidies decline.

These are proposed product commitments, not claims that their individual
components are novel or that Commonbeam is already more capable than existing
networks.

## How it would work

1. Operators register model-serving capacity and pass independent checks.
2. The network conservatively allocates usable capacity among public,
   contributor, and paid pools.
3. A user-selected router obtains admission and sends a request to an eligible
   provider.
4. The provider streams the response and supplies signed accounting evidence.
5. Auditing, settlement, and disputes determine eligible rewards.

Inference runs off-chain. A ledger can track allocations, commitments, deposits,
and settlement; LLM responses must not determine financial consensus.

Start with complete model replicas, one approved model/runtime profile, and
self-hostable clients and routers. Evaluate existing compute networks before
building a new provider network. Use contribution credits and test assets before
considering a transferable network coin.

## Non-goals

- Unlimited free inference or guaranteed perpetual access.
- Making every network participant execute or vote on every response.
- Replacing financial consensus with arbitrary AI computation.
- Treating generated text, traffic volume, or token issuance as proof of value.
- Claiming ordinary remote inference is private from the executing provider.
- Launching a speculative token before validating service and economics.

## Design and delivery

- [Proposed architecture and economics](docs/design.md)
- [Phased implementation plan and acceptance gates](docs/implementation-plan.md)

The first milestone is a threat model, capacity-and-cost model, and integration
spike. A successful MVP must demonstrate independent provider choice, bounded
public access, contribution accounting, and measurable verification behavior.

## Project decisions still open

Runtime and model selection, implementation languages, infrastructure partners,
settlement chain, credential mechanisms, governance, and issuance parameters
remain undecided. The design documents distinguish proposals from commitments.

No license has been selected yet. An explicit repository license and model
license review are prerequisites for accepting external contributions or
distributing an implementation; public visibility alone does not grant those
rights.
