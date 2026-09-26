import { Loading } from "../../components/Loading";
import { loadStorageProtection } from "../../data/storageProtection";
import { useLoaded } from "../../state/useLoaded";

// Shown inside the local-data section so the answer to "who else can read this"
// sits next to export and erase, where someone is already thinking about it.

export function StorageProtectionPanel() {
  const { data: report } = useLoaded(loadStorageProtection);

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
              <small>{file.note ?? "restricted to your account"}</small>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
