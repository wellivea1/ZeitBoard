# ADR 0035: Local file protection, and why the database is not encrypted

- Status: accepted
- Date: 2026-08-10
- Corrects a claim in [`privacy.md`](../privacy.md) that was false for the local
  store from the beginning.
- Generalises the descriptor protection from
  [ADR-0028](0028-desktop-local-agent-endpoint.md) to every private file.

## Context

`privacy.md` said, of local storage: *"At-rest encryption (local and on the
instance) and TLS in transit are required, not optional."* The instance half was
true and tested — the server seals payloads with operator-held AES-256-GCM. The
local half was never true. The desktop SQLite database holding every recorded
sleep episode was not encrypted, and the roadmap's small-debts list said so in
one line while the privacy document said the opposite in bold.

The weaker fallback the document leaned on — *"file permissions restricted to the
owner"* — was also not true on Windows, which is the only platform this app ships
on. Every private file was created with an `0o600` mode argument. On Windows that
sets the read-only attribute and leaves the inherited DACL untouched. Measured on
a development machine, the real ZeitBoard data directory granted access to five
trustees, including `SYSTEM` and `BUILTIN\Administrators`, with inheritance
enabled.

The project already knew this. ADR-0028 says, of the local agent's descriptor:
*"the restrictive-permissions claim has to be enforced with a real DACL or not
claimed at all."* That fix was applied to one file and to nothing else — not the
bearer token for the user's own server, not the settings files, not the exports,
and not the database.

## Decision

**1. One mechanism, applied everywhere.** `core/platform/privatefile` restricts a
path to the account running the process: a protected DACL with a single grant on
Windows, the file mode elsewhere. `sqlite.Open` applies it to the database and
its write-ahead log and shared-memory companions; the desktop applies it to the
data directory (so anything created later inherits it), the backend token, the
sync configuration, the settings files, and every staged export. The local agent
now calls the shared package rather than its own copy.

**2. Permissions are read back, not assumed.** The package exposes `Describe`,
which reports the DACL entries and whether inheritance is still enabled, and the
tests assert against that rather than against a nil error. A test that only
checks `os.Chmod` returned nil is a test that passes while the file is exposed —
which is how the previous claim survived. One test asserts `Describe` can report
an *unprotected* file as unprotected, so the rest cannot pass vacuously.

**3. The user is told what this is and is not.** Settings → Local data reports
which files were checked and states plainly that they are restricted, **not
encrypted**: another account on this machine cannot read them; anyone who reads
the disk from another operating system, and any program running as this user,
still can.

**4. Whole-database encryption is not shipped, and the reason is recorded.** The
driver is `modernc.org/sqlite`, a CGo-free port chosen deliberately — this
project builds and tests without a C toolchain. It exposes no VFS registration
and no serialize/deserialize hook, so there is no place to put a page-level
cipher. SQLCipher and the `go-sqlcipher` wrapper both require CGO, which would
change what the project can build and how it is verified on every platform in
CI.

The alternatives were considered and rejected:

- **Decrypt to a working file on open, re-encrypt on close.** The plaintext
  database then exists on disk for as long as the app runs, and this is a tray
  application that runs continuously. Describing that as "encrypted at rest"
  would be the same kind of overclaim being corrected here.
- **Column encryption.** Sealing the free-text columns while leaving `start_at`
  and `end_at` queryable protects almost nothing for this product: the timing
  *is* the health data. `privacy.md` already makes exactly this argument about
  the analysis-run digests — *"a digest still answers 'did they sleep at 04:12 on
  Tuesday?'"* Encrypting the note and not the time would be theatre. Encrypting
  the times breaks every range query the estimator depends on.

## Consequences

The gap between what the documents claim and what the software does is closed by
moving the claim, not by pretending. Someone reading `privacy.md` now learns that
their local sleep history is restricted to their account and unencrypted, which
is true, and can decide about full-disk encryption on that basis — a decision the
previous wording took away from them by answering it wrongly.

The protection that now exists is real and verified: on a shared or managed
Windows machine, another user account and the administrators group can no longer
read the database, the write-ahead log, the backend bearer token, the settings,
or an export.

What remains open is whole-database encryption. It needs either upstream VFS
support in the pure-Go driver, or a decision to accept CGO for the desktop build
with everything that implies for the build matrix. Neither is a small change, and
neither should be made to satisfy a sentence in a document.
