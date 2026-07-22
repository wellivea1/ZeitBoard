import { useRef, useState, type ChangeEvent } from "react";
import {
  importCalDAVCalendar,
  importCalendarFile,
  previewCalDAVCalendar,
  previewCalendarFile,
  type CalendarImportReport,
} from "../data/calendar";

const maxCalendarFileBytes = 8 << 20;

function ImportReport({ report }: { report: CalendarImportReport }) {
  return (
    <section className="calendar-import-report" aria-live="polite">
      <header>
        <strong>{report.message}</strong>
        <span>{report.coverageLabel}</span>
      </header>
      <dl>
        <div>
          <dt>Events</dt>
          <dd>{report.eventCount}</dd>
        </div>
        <div>
          <dt>Busy</dt>
          <dd>{report.busyCount}</dd>
        </div>
        <div>
          <dt>All day</dt>
          <dd>{report.allDayCount}</dd>
        </div>
      </dl>
      {report.events.length > 0 && (
        <div className="calendar-import-preview-list">
          {report.events.slice(0, 6).map((event) => (
            <div key={event.eventId}>
              <strong>{event.title}</strong>
              <span>
                {event.allDay ? "All day" : event.startLabel} - {event.busy ? "busy" : "available"}
              </span>
            </div>
          ))}
          {(report.previewTruncated || report.events.length > 6) && (
            <small>Preview is abbreviated; commit uses the complete source.</small>
          )}
        </div>
      )}
    </section>
  );
}

interface CalendarFileSelection {
  fileName: string;
  contents: string;
}

function CalendarFileForm({
  available,
  busy,
  file,
  report,
  onChoose,
  onRun,
}: {
  available: boolean;
  busy: boolean;
  file: CalendarFileSelection | null;
  report: CalendarImportReport | null;
  onChoose: (event: ChangeEvent<HTMLInputElement>) => void;
  onRun: (commit: boolean) => void;
}) {
  return (
    <div className="calendar-import-form" role="tabpanel">
      <label>
        <span>iCalendar file</span>
        <input
          type="file"
          accept=".ics,text/calendar"
          disabled={!available || busy}
          onChange={onChoose}
        />
      </label>
      {file && <small>{file.fileName} - contents stay in this desktop process</small>}
      <div className="calendar-import-actions">
        <button
          className="button secondary compact"
          type="button"
          disabled={!available || !file || busy}
          onClick={() => onRun(false)}
        >
          {busy ? "Reading..." : "Preview"}
        </button>
        <button
          className="button primary compact"
          type="button"
          disabled={
            !available || !file || busy || !report || report.imported || report.kind !== "ics"
          }
          onClick={() => onRun(true)}
        >
          Import snapshot
        </button>
      </div>
    </div>
  );
}

interface CalDAVFields {
  endpoint: string;
  label: string;
  username: string;
  password: string;
}

function CalDAVForm({
  available,
  busy,
  fields,
  report,
  onChange,
  onRun,
}: {
  available: boolean;
  busy: boolean;
  fields: CalDAVFields;
  report: CalendarImportReport | null;
  onChange: (field: keyof CalDAVFields, value: string) => void;
  onRun: (commit: boolean) => void;
}) {
  const canRequest = available && Boolean(fields.endpoint.trim()) && !busy;
  return (
    <div className="calendar-import-form" role="tabpanel">
      <label>
        <span>Collection URL</span>
        <input
          type="url"
          value={fields.endpoint}
          placeholder="https://calendar.example/dav/user/calendar/"
          disabled={!available || busy}
          onChange={(event) => onChange("endpoint", event.currentTarget.value)}
        />
      </label>
      <label>
        <span>Local label</span>
        <input
          value={fields.label}
          placeholder="Personal calendar"
          disabled={!available || busy}
          onChange={(event) => onChange("label", event.currentTarget.value)}
        />
      </label>
      <div className="calendar-credential-row">
        <label>
          <span>Username</span>
          <input
            autoComplete="username"
            value={fields.username}
            disabled={!available || busy}
            onChange={(event) => onChange("username", event.currentTarget.value)}
          />
        </label>
        <label>
          <span>One-shot password</span>
          <input
            type="password"
            autoComplete="current-password"
            value={fields.password}
            disabled={!available || busy}
            onChange={(event) => onChange("password", event.currentTarget.value)}
          />
        </label>
      </div>
      <small>The password is cleared after each request and is never stored.</small>
      <div className="calendar-import-actions">
        <button
          className="button secondary compact"
          type="button"
          disabled={!canRequest}
          onClick={() => onRun(false)}
        >
          {busy ? "Connecting..." : "Preview REPORT"}
        </button>
        <button
          className="button primary compact"
          type="button"
          disabled={!canRequest || !report || report.imported || report.kind !== "caldav"}
          onClick={() => onRun(true)}
        >
          Fetch and import
        </button>
      </div>
    </div>
  );
}

export function CalendarImportPanel({
  available,
  zoneId,
  onChanged,
}: {
  available: boolean;
  zoneId: string;
  onChanged: () => void;
}) {
  const [mode, setMode] = useState<"file" | "caldav">("file");
  const [file, setFile] = useState<CalendarFileSelection | null>(null);
  const [caldav, setCalDAV] = useState<CalDAVFields>({
    endpoint: "",
    label: "",
    username: "",
    password: "",
  });
  const [report, setReport] = useState<CalendarImportReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const fileReadVersion = useRef(0);

  const selectMode = (next: "file" | "caldav") => {
    setMode(next);
    setReport(null);
    setError("");
    setCalDAV((current) => ({ ...current, password: "" }));
  };

  const chooseFile = (event: ChangeEvent<HTMLInputElement>) => {
    const version = ++fileReadVersion.current;
    const selected = event.currentTarget.files?.[0];
    setReport(null);
    setError("");
    if (!selected) {
      setFile(null);
      return;
    }
    if (!selected.name.toLowerCase().endsWith(".ics")) {
      setFile(null);
      setError("Select an .ics calendar file.");
      return;
    }
    if (selected.size > maxCalendarFileBytes) {
      setFile(null);
      setError("Calendar files must be 8 MiB or smaller.");
      return;
    }
    void selected.text().then(
      (contents) => {
        if (fileReadVersion.current === version) {
          setFile({ fileName: selected.name, contents });
        }
      },
      () => {
        if (fileReadVersion.current === version) {
          setError("The selected file could not be read.");
        }
      },
    );
  };

  const changeCalDAV = (field: keyof CalDAVFields, value: string) => {
    setCalDAV((current) => ({ ...current, [field]: value }));
    if (field === "endpoint" || field === "username") setReport(null);
  };

  const runFile = (commit: boolean) => {
    if (!file || busy) return;
    setBusy(true);
    setError("");
    const operation = commit ? importCalendarFile : previewCalendarFile;
    void operation({ ...file, zoneId }).then(
      (next) => {
        setBusy(false);
        setReport(next);
        if (commit) {
          setFile(null);
          onChanged();
        }
      },
      (reason: unknown) => {
        setBusy(false);
        setError(reason instanceof Error ? reason.message : "Calendar file import failed.");
      },
    );
  };

  const runCalDAV = (commit: boolean) => {
    if (!caldav.endpoint.trim() || busy) return;
    setBusy(true);
    setError("");
    const operation = commit ? importCalDAVCalendar : previewCalDAVCalendar;
    void operation({
      endpoint: caldav.endpoint.trim(),
      label: caldav.label.trim(),
      username: caldav.username.trim(),
      password: caldav.password,
      zoneId,
    }).then(
      (next) => {
        setBusy(false);
        setCalDAV((current) => ({ ...current, password: "" }));
        setReport(next);
        if (commit) onChanged();
      },
      (reason: unknown) => {
        setBusy(false);
        setCalDAV((current) => ({ ...current, password: "" }));
        setError(reason instanceof Error ? reason.message : "CalDAV import failed.");
      },
    );
  };

  return (
    <section className="calendar-import-panel" aria-labelledby="calendar-import-title">
      <header>
        <p className="section-kicker">Read-only adapters</p>
        <h2 id="calendar-import-title">Add calendar source</h2>
      </header>
      <div className="calendar-import-tabs" role="tablist" aria-label="Calendar source type">
        <button
          type="button"
          role="tab"
          aria-selected={mode === "file"}
          onClick={() => selectMode("file")}
        >
          ICS file
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={mode === "caldav"}
          onClick={() => selectMode("caldav")}
        >
          CalDAV
        </button>
      </div>

      {!available && (
        <p className="inline-notice">Import controls require the ZeitBoard desktop service.</p>
      )}

      {mode === "file" ? (
        <CalendarFileForm
          available={available}
          busy={busy}
          file={file}
          report={report}
          onChoose={chooseFile}
          onRun={runFile}
        />
      ) : (
        <CalDAVForm
          available={available}
          busy={busy}
          fields={caldav}
          report={report}
          onChange={changeCalDAV}
          onRun={runCalDAV}
        />
      )}

      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {report && <ImportReport report={report} />}
    </section>
  );
}
