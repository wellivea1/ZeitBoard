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
  "enrollmentSecretFile": "secrets/enrollment-secret.txt"
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

Do not commit key files, enrollment secrets, device tokens, or real config files.

## Generate Secrets

PowerShell:

```powershell
$bytes = New-Object byte[] 32
[Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
[Convert]::ToBase64String($bytes) | Set-Content -NoNewline secrets\data-key.txt

$enrollment = New-Object byte[] 32
[Security.Cryptography.RandomNumberGenerator]::Fill($enrollment)
[Convert]::ToBase64String($enrollment) | Set-Content -NoNewline secrets\enrollment-secret.txt
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

## Backups

Back up the data directory and the data key. Without the data key, encrypted payloads
cannot be recovered. Backups should be encrypted independently and stored separately
from the live host. SQLite sidecar files such as `-wal` and `-shm` are part of the data
directory and must be protected too.

## Network And Telemetry

Milestone 1 makes no outbound calls and has no telemetry path. The only network surface
is the TLS listener configured by the operator. Future BYOK LLM provider traffic is
out of scope for M1 and requires its own review before implementation.
