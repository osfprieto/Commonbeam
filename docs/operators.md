# Operator and client guide

Everything below is for a local test network. No step grants production security,
an inference entitlement, or a real cryptocurrency balance.

## Local native demonstration

Install the pinned Go toolchain and provide a dedicated local PostgreSQL
database. Set `COMMONBEAM_DATABASE_URL` through your shell or secret store, not
source files. Loopback-only test PostgreSQL may use `sslmode=disable`.

```powershell
go build -o bin\commonbeam.exe .\cmd\commonbeam
$env:COMMONBEAM_DATABASE_URL = "<your local PostgreSQL connection URL>"
.\bin\commonbeam.exe doctor
.\bin\commonbeam.exe dev
```

The first start initializes `.commonbeam`, enrolls two simulated providers, and
assigns 50,000 public, 110,000 contributor and 40,000 test-paid units. These are
declared simulation units, not measured GPU capacity. The seed occurs once.
The first enrolled provider establishes one expiring epoch. To evaluate another
model or a new epoch, create an explicitly new state directory and database.

Use `--state` for an alternate state directory. For non-default ports, initialize
first with `init --authority http://127.0.0.1:PORT --router http://127.0.0.1:PORT`.
Never overwrite an existing identity to work around an evidence-key error.

In another terminal, run `session`, `models`, `chat`, `balance`, and `status`.
All accept `--state`; `chat` reads stdin when `--prompt` is absent.
`chat --router http://127.0.0.1:PORT` explicitly selects another participating
router without discarding the saved session.

## Container definition

Docker must be available. The definition has been syntax checked; verify actual
container behavior on an equipped host before claiming that path qualified.

Generate and retain a private ignored `.env` file once:

```powershell
if (!(Test-Path .env)) {
  $password = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(24))
  "COMMONBEAM_POSTGRES_PASSWORD=$password" | Set-Content -Encoding ascii .env
}
docker compose --env-file .env -f deploy\compose.yaml up --build
```

The database has no published host port; router and authority ports publish only
on host loopback. `--container-network` enables internal container binding, not
permission for internet exposure. Retain the same password and named volumes
across restarts.

For a quick in-container client:

```powershell
docker compose --env-file .env -f deploy\compose.yaml exec commonbeam /commonbeam session --state /state
docker compose --env-file .env -f deploy\compose.yaml exec commonbeam /commonbeam chat --state /state --prompt "Hello"
```

`docker compose ... down` stops these services while preserving state. Do not
remove volumes to solve a configuration mismatch without intentionally
discarding that test network.

## Independent consumers

The network operator shares only `network.json` and the public key fingerprint,
not `secrets.json`, evidence keys or provider keys. Obtain the fingerprint
independently of an untrusted descriptor.

```powershell
.\bin\commonbeam.exe join --state .consumer --descriptor "<descriptor file>" --fingerprint "<trusted fingerprint>"
.\bin\commonbeam.exe session --state .consumer
.\bin\commonbeam.exe chat --state .consumer --prompt "Hello"
```

A session is a pseudonymous finite account, not proof of a unique human.
Creating more sessions consumes the same finite public funding; no fairness
guarantee is implied. Session exhaustion does not replenish the pool.
The CLI retains an existing session by default. `session --new-session`
explicitly replaces it; this discards local access to the prior allowance.

For API clients, create a session at `POST /commonbeam/v1/session`, then use its
token as a bearer credential with the selected router's `/v1` API base.
Do not print or commit the token. Supported requests are text chat, bounded
output, optional streaming and temperature zero. Unknown fields fail explicitly.

Examples:

```powershell
python -m pip install -r examples\python\requirements.txt
python examples\python\chat.py
node examples\javascript\chat.mjs
```

The Python example uses the OpenAI client with automatic retries disabled.
The JavaScript example uses native fetch. Both default to loopback and obtain
bounded test sessions; neither hardcodes an external model company's endpoint.
Override `COMMONBEAM_ROUTER_URL`, `COMMONBEAM_SESSION_TOKEN`, and
`COMMONBEAM_MODEL` as needed.

After an ambiguous or interrupted request, query `job --job ID` and use the same
`Idempotency-Key` through the HTTP API rather than silently executing again.
`cancel --job ID` requests cancellation; provider acknowledgement or lease expiry
releases accepted work. Partial/failed pilot work is refunded, not billed as
real money.

## Independent routers and providers

Join the descriptor first. Supply only the needed scoped credential:

- Router: `COMMONBEAM_ROUTER_TOKEN`; start `router --state DIR --listen IP:PORT`.
- New provider: `COMMONBEAM_ENROLL_TOKEN`; start `provider --state DIR --name NAME`.
- Sponsor: `COMMONBEAM_SPONSOR_TOKEN`.
- Administrator: `COMMONBEAM_ADMIN_TOKEN`.

An enrolled provider saves its own key and token; it does not need the enrollment
token to restart. The worker polls outbound, obeys deadlines and sends signed
receipts. No inbound provider port or peer-supplied backend URL is used.
`drain --name NAME` stops new assignments; accepted work remains bounded.
Restarting the provider explicitly resumes unexpired enrollment.

`contribute --name NAME` exchanges one enrolled provider's eligible test
contribution for a one-time allowance from the contributor pool and saves it
as the current session. It does not prove hardware, issue currency, or earn a
financial yield. The enrollment authority remains trusted to limit fabricated
providers.
If a session is already selected, `contribute` requires `--new-session` to
explicitly switch away from that allowance instead of silently discarding it.

To attach an already approved real server, use a separate network/database and:

```powershell
.\bin\commonbeam.exe provider --state DIR --name NAME --backend vllm --backend-url http://127.0.0.1:8000/v1 --model "<exact ID>" --model-hash "<SHA-256>"
```

`--backend llamacpp` uses the same probed HTTP interface with a distinct
self-attested profile. The server must supply valid streaming usage and enforce
its own memory/context limits. Optional backend authentication comes from
`COMMONBEAM_BACKEND_API_KEY`. Only loopback IP literals are accepted; redirects
are disabled. A declared digest is not remote attestation.

No automatic runtime/model installation is provided. License acceptance,
hardware/resource consent, runtime pinning and actual measurements precede
qualification. Real-model and GPU behavior are not established by simulation.

## Sponsors, administrators and auditors

`sponsor --pool POOL --units N` adds test funding only when unallocated declared
capacity covers it. It is not a real payment. The initial demo has allocated its
entire declared budget, so additional funding requires eligible new capacity.

`sponsor --from POOL --pool OTHER --units N` rebalances only unallocated
balances. Reclaim cannot revoke existing accounts or accepted work; there is no
automatic loan or preemption promise.

`grant --pool contributor|paid --units N` is an administrator test-account
operation. The response includes a private session token; deliver it securely
or save it as a private `session.json`. Grants transfer existing balances rather
than creating new supply.

`revoke --provider-id ID` invalidates that provider's credential and stops new
assignments. Accepted work is resolved through bounded lease reconciliation;
revocation does not mint replacement capacity.

`audit --job ID` checks authorized retained evidence. Roughly one quarter of
successful jobs are independently randomly marked after completion for audit
selection; the CLI also permits manual observation. No automatic penalty or
committee settlement is implemented. Signature validity, matching output
commitments, simulation replay and proof of model execution are different facts.

## Persistence, recovery and privacy

Back up the dedicated PostgreSQL database using its native dump/restore tools,
and independently protect the state directory and evidence key. Never commit
backups or keys. Restore the same network identity and key; startup refuses
mismatches. Windows file-mode bits alone are not a complete ACL policy, so
protect the state directory through the operating system.

Jobs and reservations persist across process restarts. A lost provider job is
not automatically rerun: the fixed deadline refunds unresolved test work.
PostgreSQL transactions and a shared accounting lock prevent separate routers
from creating duplicate holds.

Terminal prompts/output are logically cleared after the retention window;
receipts and accounting remain. Backups and WAL may retain older encrypted
evidence. The authority possesses the evidence key, and routers/providers see
plaintext during execution. None of this is confidential inference.

## Qualification still required

`doctor` currently reports basic platform/tool/configuration prerequisites, not
full hardware certification. Real model throughput/reproducibility, detailed
driver/memory/disk diagnostics, Docker execution, broad platform runtime tests,
database restore drills, sustained overload, independent operators, production
economics, and internet-safe deployment remain explicit gates.
