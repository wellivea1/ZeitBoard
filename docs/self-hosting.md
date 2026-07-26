# Self-Hosting Runbook

ZeitBoard's server is a Go daemon that the user or their chosen operator runs for their
own devices. The project operates no hosted service and receives no telemetry.

## What The Server Stores

Milestone 1 stores an append-only sync log for the user's own observations and
corrections. Payload bytes are encrypted at rest with AES-256-GCM using an
operator-supplied 32-byte data key. SQLite metadata still includes record IDs, device
IDs, record kinds, sequence numbers, timestamps, token hashes, nonces, ciphertext, and
payload hashes, so the entire data directory should be treated as sensitive.

Trusted-view sharing remains separate: it uses default-deny projection DTOs and is not
the device-sync format.

## Configuration

The daemon accepts a JSON config file via `-config` or `ZEITBOARD_CONFIG`. Environment
variables override file values.

```json
{
  "listenAddress": "127.0.0.1:8765",
  "tlsCertPath": "",
  "tlsKeyPath": "",
  "dataDir": "data",
  "dataKeyFile": "secrets/data-key.txt",
  "enrollmentSecretFile": "secrets/enrollment-secret.txt",
  "assistant": {
    "provider": "disabled",
    "model": "",
    "apiKeyFile": "",
    "endpoint": ""
  }
}
```

Keys:

| Setting | Environment variable | Required | Notes |
| --- | --- | --- | --- |
| `listenAddress` | `ZEITBOARD_LISTEN_ADDR` | No | Defaults to `127.0.0.1:8765`. |
| `tlsCertPath` | `ZEITBOARD_TLS_CERT` | For non-local binds | PEM certificate path. |
| `tlsKeyPath` | `ZEITBOARD_TLS_KEY` | For non-local binds | PEM private-key path. |
| `dataDir` | `ZEITBOARD_DATA_DIR` | No | Directory for `zeitboardd.db` and SQLite sidecar files. |
| `dataKeyFile` | `ZEITBOARD_DATA_KEY_FILE` | Yes, unless env key is set | File containing a 32-byte key encoded as base64, raw-url base64, hex, or raw 32 bytes. |
| n/a | `ZEITBOARD_DATA_KEY` | Yes, unless key file is set | The at-rest encryption key value. |
| `enrollmentSecretFile` | `ZEITBOARD_ENROLLMENT_SECRET_FILE` | Yes, unless env secret is set | File containing the device enrollment secret. |
| n/a | `ZEITBOARD_ENROLLMENT_SECRET` | Yes, unless secret file is set | Secret used only to enroll new devices. |
| `assistant.provider` | `ZEITBOARD_LLM_PROVIDER` | No | `disabled`, `openai`, `anthropic`, `openrouter`, or `opencode_zen`. |
| `assistant.model` | `ZEITBOARD_LLM_MODEL` | Required when a provider is enabled | Provider model name. |
| `assistant.apiKeyFile` | `ZEITBOARD_LLM_API_KEY_FILE` | Required when provider key env is absent | File containing the provider API key. |
| n/a | `ZEITBOARD_LLM_API_KEY` | Required when a provider is enabled and no key file is set | Provider API key; never returned by status APIs. |
| `assistant.endpoint` | `ZEITBOARD_LLM_ENDPOINT` | Required for `opencode_zen`, optional override otherwise | Plain HTTPS endpoint for the provider transport. |

Relative file and directory paths in a JSON config resolve from the directory
containing that config file. Relative paths supplied through environment
variables retain normal process-working-directory semantics; use absolute paths
for service deployments.

Do not commit key files, enrollment secrets, device tokens, provider keys, or real config
files.

## Generate Secrets

PowerShell:

```powershell
New-Item -ItemType Directory -Force secrets | Out-Null
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
try {
    $bytes = New-Object byte[] 32
    $rng.GetBytes($bytes)
    [Convert]::ToBase64String($bytes) | Set-Content -NoNewline -Encoding ASCII secrets\data-key.txt

    $enrollment = New-Object byte[] 32
    $rng.GetBytes($enrollment)
    [Convert]::ToBase64String($enrollment) | Set-Content -NoNewline -Encoding ASCII secrets\enrollment-secret.txt
}
finally {
    $rng.Dispose()
}
```

OpenSSL:

```bash
mkdir -p secrets
openssl rand -base64 32 > secrets/data-key.txt
openssl rand -base64 32 > secrets/enrollment-secret.txt
```

## TLS

For local development, binding to `127.0.0.1` or `localhost` without certificate paths
uses a runtime-generated self-signed localhost certificate. Clients must explicitly
trust or skip verification for that development setup.

For any non-local listen address, provide `tlsCertPath` and `tlsKeyPath`. Use normal
certificate rotation and host hardening for the machine running the daemon.

## Run

From `apps/server`:

```powershell
go run ./cmd/zeitboardd -config .\config.local.json
```

or:

```bash
go run ./cmd/zeitboardd -config ./config.local.json
```

Health check:

```bash
curl -k https://127.0.0.1:8765/healthz
```

Enroll a device:

```bash
curl -k https://127.0.0.1:8765/v1/devices \
  -H 'Content-Type: application/json' \
  -d '{"enrollmentSecret":"<secret from operator>","label":"desktop"}'
```

The response contains a per-device bearer token. Store it in that device's local secret
storage. The server stores only a hash of the token.

## Native Windows Service

From the repository root, use an elevated Windows PowerShell prompt:

```powershell
.\scripts\installer\install-server.ps1
```

The installer defaults to `%PROGRAMDATA%\ZeitBoard`, generates secrets only
when absent, restricts the managed root to SYSTEM and Administrators, validates
the staged config before changing service registration, and starts a delayed
automatic service named `ZeitBoardServer`. The daemon uses the native Windows
SCM lifecycle: it reports Running only after its listener is established and
performs a bounded graceful shutdown on Stop or system shutdown.

A custom `-InstallRoot` must be a dedicated service directory outside Desktop,
Documents, and Downloads, and it must not contain junctions or symbolic links;
the installer replaces ACL inheritance recursively and fails closed otherwise.

Loopback is the default. A non-loopback `-ListenAddress` requires both
`-TlsCertPath` and `-TlsKeyPath`. `-Firewall` creates a Private-profile inbound
rule tied to the managed daemon executable; the installer preserves same-named
rules that point elsewhere. Service logs rotate at 10 MiB with one `.1` backup.
TLS paths passed on the command line are normalized to absolute paths; relative
paths already stored in `config.json` resolve from that file's directory.
The public availability portal is not implemented or exposed by this command.

Rerunning the command upgrades an owned service and restores its prior files,
registration, and running/stopped state if publication or startup fails. It
refuses to modify a same-named service owned by another executable.

To remove the desktop installation and owned service registration while
preserving the server root:

```powershell
.\scripts\installer\uninstall.ps1 -RemoveServer
```

## Backups

Back up the data directory and the data key. Without the data key, encrypted payloads
cannot be recovered. Backups should be encrypted independently and stored separately
from the live host. SQLite sidecar files such as `-wal` and `-shm` are part of the data
directory and must be protected too.

## Assistant Provider Traffic

With `assistant.provider` set to `disabled`, assistant requests do not make provider
network calls. When the operator configures OpenAI, Anthropic, OpenRouter, or OpenCode
Zen, the daemon sends only the assistant's minimized redacted context to that provider
using the operator's key. Provider credentials are not returned by `/v1/status`, are not
placed in model context, and are not written to fixtures.

## Network And Telemetry

The project has no telemetry path. The daemon listens on the TLS address configured by
the operator. Outbound network calls occur only when the operator enables a BYOK
assistant provider; arbitrary web access is not exposed to the assistant path.

## Voice Via An MCP Client (Claude Desktop)

ZeitBoard ships no speech stack (ADR-0006): live voice comes from an MCP-capable
client driving the local connector. With Claude Desktop:

1. Build the connector: `go build ./cmd/zeitboard-mcp` (in `apps/server`).
2. Register it in Claude Desktop's MCP configuration as a stdio server, pointing it
   at your instance URL and a device token you enrolled for it (same enrollment flow
   as any device; revoke it like any device).
3. Use Claude Desktop's voice mode. The connector exposes only allowlisted read
   projections and propose-only actions with call budgets — there is no approve or
   apply tool (ADR-0012), so spoken requests end as pending proposals you approve in
   the app.

What the agent can see is the same redacted, speakable projection surface the UI
uses. Which *model* hears it is the client's configuration and the user's provider
relationship — the same BYOK posture as the in-app assistant. Cloud skill packaging
remains future work behind its own privacy review.
