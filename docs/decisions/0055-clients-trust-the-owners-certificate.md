# ADR 0055: Clients trust the owner's server by its certificate

- Status: accepted
- Date: 2026-10-11
- Implements the roadmap's small debt "prefer a CA-cert path over localhost skip-verify for the
  MCP client". Builds on [ADR-0009](0009-backend-sync-design.md) and
  [ADR-0012](0012-mcp-agent-connector.md).

## Context

A ZeitBoard client verifies the owner's server against the system's public roots. A server with a
public certificate works. Many self-hosters use something else:

- a certificate from their own authority;
- a self-signed certificate on the home network;
- the daemon's own localhost certificate during development.

Such a server could not be reached at all, except by skipping verification. The desktop allows
skipping only for localhost. The MCP connector allowed it for any address. Pinning the daemon's
localhost certificate was not possible either, because the daemon made a new one on every start.

## Decision

1. **A client may trust one more certificate for the owner's server.** That is the server's own
   certificate, or the authority that issued it, given as PEM or DER. It is added to the system's
   roots for that client's connections to the server and nothing else. Verification stays on: the
   server must still present a valid certificate for the address the client was given.
   `core/tlstrust` reads the certificate, builds the trust and gives the SHA-256 fingerprint a
   person checks it by.
   - **Desktop.** Settings › Sync has a "Server certificate" row. The owner chooses a file and sees
     its name, its expiry and its full fingerprint. The certificate is kept with the sync settings.
     It is public, so the status carries it, and enrolling again keeps it unless the owner removes
     it.
   - **MCP connector.** `ZEITBOARD_MCP_SERVER_CERTIFICATE_FILE` names the file.
   - **Phone.** The companion's enrollment form takes a certificate file, through the system's
     file picker, and shows the same name, expiry and fingerprint. The certificate is kept with the
     enrollment and used only for that server's connections.
2. **The daemon keeps its localhost certificate.** For a localhost bind without certificate paths,
   it keeps the certificate in `<dataDir>/tls/`, readable by the owner only. It reuses the
   certificate until a month before it expires, then makes a new one. At start it logs the path
   and the fingerprint, so a local client can trust the file instead of skipping verification.
3. **Skipping verification is for localhost only, everywhere.** The desktop already refused it for
   any other host. The MCP connector now refuses it too and names the certificate setting instead.
   It stays for localhost development, so existing development setups keep working.

## Consequences

- A server on a private network, with the owner's own authority or a self-signed certificate,
  is reached with verification on.
- A client that trusts the daemon's localhost certificate needs the new file after a renewal, once
  a year. The renewal happens a month before expiry, and the daemon's log names it.
- Choosing the wrong certificate does not weaken anything. The connection fails as it would
  without one.
- The fingerprint is shown whole. Checking it against the daemon's log is the owner's step; the
  app cannot know which certificate the owner meant.
