import { useState } from "react";
import { Loading } from "../../components/Loading";
import {
  loadStorageProtection,
  setStorageEncryption,
  type StorageEncryption,
  type StorageProtectionFile,
} from "../../data/storageProtection";
import { useLoaded } from "../../state/useLoaded";

// Shown inside the local-data section so the answer to "who else can read this"
// sits next to export and erase, where someone is already thinking about it.

function fileNote(file: StorageProtectionFile) {
  const access = file.note ?? "restricted to your account";
  return file.encrypted ? `${access}, encrypted by Windows` : access;
}

function encryptionState(encryption: StorageEncryption) {
  if (encryption.pending) {
    return encryption.chosen
      ? "Windows encrypts the files when ZeitBoard next starts. Quit it from the tray and open it again."
      : "Windows decrypts the files when ZeitBoard next starts.";
  }
  if (encryption.files > 0 && encryption.encryptedFiles === encryption.files) {
    return `Windows encrypts all ${encryption.files} files.`;
  }
  return "Not encrypted.";
}

// Where ZeitBoard cannot encrypt its files itself, the system's own disk
// encryption is the way, named as each system names it.
function noEncryptionCopy(platform: string) {
  switch (platform) {
    case "windows":
      return "This edition of Windows cannot encrypt files; that comes with Pro, Enterprise and Education. Device encryption or BitLocker, in Windows Settings, encrypts the whole disk instead.";
    case "macos":
      return "ZeitBoard encrypts its files only on Windows. On a Mac, FileVault, in System Settings › Privacy & Security, encrypts the whole disk, and these files with it.";
    default:
      return "ZeitBoard encrypts its files only on Windows. Here, encrypting the whole disk, which most systems offer when they are installed, protects these files with everything else.";
  }
}

// Windows' file encryption (ADR-0063). What it protects, what it does not, and
// how the files could be lost are said before the choice is made.
function StorageEncryptionControl({
  encryption,
  onChoose,
  busy,
  error,
}: {
  encryption: StorageEncryption;
  onChoose: (encrypt: boolean) => void;
  busy: boolean;
  error: string;
}) {
  if (!encryption.supported) {
    return (
      <div className="storage-encryption">
        <h4>{encryption.platform === "windows" ? "Encrypted by Windows" : "Encryption"}</h4>
        <p className="settings-copy">{noEncryptionCopy(encryption.platform)}</p>
      </div>
    );
  }
  const encrypting = encryption.chosen;
  return (
    <div className="storage-encryption">
      <h4>Encrypted by Windows</h4>
      <p className="settings-copy">
        Windows can encrypt ZeitBoard&apos;s folder with a key kept in your Windows account. Other
        accounts, administrators and other operating systems then see only noise; programs running
        as you still read the files. The first time, Windows offers to back up that key. Take the
        backup and keep it away from this computer: without it, a password reset by an
        administrator, or losing this account, leaves the files unreadable. Exports you save
        elsewhere are not encrypted.
      </p>
      <p className="storage-encryption-state" role="status">
        {encryptionState(encryption)}
      </p>
      <button
        type="button"
        className="button secondary"
        disabled={busy}
        onClick={() => onChoose(!encrypting)}
      >
        {encrypting ? "Stop encrypting" : "Encrypt with Windows"}
      </button>
      {(error || encryption.problem) && (
        <p className="form-error" role="alert">
          {error || encryption.problem}
        </p>
      )}
    </div>
  );
}

export function StorageProtectionPanel() {
  const loaded = useLoaded(loadStorageProtection);
  const report = loaded.data;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const choose = (encrypt: boolean) => {
    setBusy(true);
    setError("");
    void setStorageEncryption(encrypt).then(
      (next) => {
        loaded.set(next);
        setBusy(false);
      },
      (reason: unknown) => {
        setError(reason instanceof Error ? reason.message : "The choice could not be saved.");
        setBusy(false);
      },
    );
  };

  return (
    <section
      className="data-control-card storage-protection"
      aria-labelledby="storage-protection-title"
    >
      <div>
        <h3 id="storage-protection-title">Who can read these files</h3>
        {report === undefined ? (
          <Loading>Checking file permissions…</Loading>
        ) : (
          <p
            className="storage-protection-headline"
            data-state={report.state}
            role={report.state === "at_risk" ? "alert" : undefined}
          >
            {report.headline}
          </p>
        )}
      </div>

      {/* Never let a file permission read as encryption. */}
      {report && <p className="settings-copy">{report.detail}</p>}

      {report && report.files.length > 0 && (
        <ul className="storage-protection-list">
          {report.files.map((file) => (
            <li key={file.name} data-ok={file.ownerOnly && !file.inherited}>
              <span>{file.name}</span>
              <small>{fileNote(file)}</small>
            </li>
          ))}
        </ul>
      )}

      {report && report.state !== "unknown" && (
        <StorageEncryptionControl
          encryption={report.encryption}
          onChoose={choose}
          busy={busy}
          error={error}
        />
      )}
    </section>
  );
}
