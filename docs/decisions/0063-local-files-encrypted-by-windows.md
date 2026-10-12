# ADR 0063: The local files encrypted by Windows, by choice

- Status: accepted
- Date: 2026-10-12
- Delivers the planned "encrypting the local database" on Windows with no new dependency.
  Amends [ADR-0035](0035-local-file-protection.md), which considered page-level ciphers, decrypted
  working copies and column encryption, but not the operating system's own file encryption.

## Context

The desktop's files carry an owner-only permission (ADR-0035). That stops other accounts on the
computer. It does not stop anyone who reads the disk from another operating system, or an
administrator. Page-level encryption inside SQLite needs a driver with an encryption hook. One
exists (ADR-0035's update), but adopting it is a new dependency and a key-custody decision the
owner has not made.

Windows has its own file encryption, EFS, in its Pro, Enterprise and Education editions. It
encrypts files transparently, with a key kept in the signed-in account, and SQLite works on it
unchanged. A program can ask for it with one call per file. The only condition is that nothing has
the file open.

## Decision

1. **A choice in Settings › Your data, off by default.** "Encrypt with Windows" appears where
   Windows can encrypt files. Elsewhere, Settings names the editions that can, and points to device
   encryption or BitLocker. Before the choice, Settings says what it protects, what it does not, and
   how the files could be lost.
2. **Applied when ZeitBoard starts.** The choice is saved beside the database. Before the store
   opens, ZeitBoard brings the data folder and every file in it in line with the choice. Folders
   are encrypted before the files in them, and decrypted after them. Choosing it also marks the
   folder at once, so files made from then on are encrypted. Settings says the rest happens at the
   next start. A file that could not be done is reported, and the next start tries again.
3. **Only ZeitBoard's own choice is acted on.** Without a saved choice, nothing is encrypted or
   decrypted. A folder the owner encrypted themselves is not ZeitBoard's to undo.
4. **Read back, as ADR-0035 requires.** Each file's state comes from Windows, not from the choice.
   The readout says the files are encrypted only once Windows says every one is.
5. **The key stays Windows'.** Windows offers to back up the key the first time it encrypts.
   Settings says to take that backup and keep it away from the computer. Without it, an
   administrator resetting the password, or a lost account, leaves the files unreadable. A copy
   synced to the owner's server would remain.

## Consequences

- Other accounts, administrators and other operating systems see only noise. Programs running as
  the owner still read the files, as with any encryption that is open while someone is signed in.
- Exports saved elsewhere are not encrypted, and Settings says so.
- Tests stand in for Windows. Encrypting for real makes a key in the account that runs the test, so
  the real round trip runs only in CI, on a throwaway Windows account.
- Not done: page-level encryption with a key ZeitBoard holds, which waits on adopting the driver
  in ADR-0035's update.
