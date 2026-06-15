import { useState } from "react";
import { ClockGlyph, MoonGlyph, ShieldGlyph } from "./components/Glyph";
import { fixtureProfiles, getFixtureProfile } from "./data/profiles";

type FixtureProfile = ReturnType<typeof getFixtureProfile>;

const profileList = Object.values(fixtureProfiles);

const dateTimeFormatter = new Intl.DateTimeFormat("en-US", {
  weekday: "short",
  month: "short",
  day: "numeric",
  hour: "numeric",
  minute: "2-digit",
});

const timeFormatter = new Intl.DateTimeFormat("en-US", {
  hour: "numeric",
  minute: "2-digit",
});

function formatDateTime(value: string) {
  return dateTimeFormatter.format(new Date(value));
}

function formatWindow(window: { earliest_at: string; latest_at: string }) {
  return `${formatDateTime(window.earliest_at)}-${timeFormatter.format(new Date(window.latest_at))}`;
}

function ProjectionCard({
  label,
  value,
  kind,
}: {
  label: string;
  value: string;
  kind: "moon" | "clock";
}) {
  return (
    <article className="projection-card">
      <span className="projection-icon">{kind === "moon" ? <MoonGlyph /> : <ClockGlyph />}</span>
      <p>{label}</p>
      <strong>{value}</strong>
    </article>
  );
}

function ActiveProfile({ profile }: { profile: FixtureProfile }) {
  const projection = profile.projection;
  if (!projection) return null;

  return (
    <>
      <section className="status-panel" aria-labelledby="status-heading">
        <div className="phase-dial" aria-hidden="true">
          <span>
            <MoonGlyph />
          </span>
        </div>
        <div>
          <p className="kicker">Synthetic trusted projection</p>
          <h1 id="status-heading">{profile.label}</h1>
          <p>
            This static view contains only the fields explicitly granted to this profile. Exact
            observations, medication, location, diagnosis, and calendar text are not present.
          </p>
          {projection.confidence && (
            <span className="confidence">{projection.confidence} confidence</span>
          )}
        </div>
      </section>

      <section className="projection-grid" aria-label="Shared planning fields">
        {projection.predicted_sleep_window && (
          <ProjectionCard
            label="Predicted sleep window"
            value={formatWindow(projection.predicted_sleep_window)}
            kind="moon"
          />
        )}
        {projection.predicted_waking_window && (
          <ProjectionCard
            label="Predicted waking window"
            value={formatWindow(projection.predicted_waking_window)}
            kind="clock"
          />
        )}
        {projection.availability?.map((window, index) => (
          <ProjectionCard
            key={`${window.start_at}-${window.end_at}`}
            label={`Availability${projection.availability!.length > 1 ? ` ${index + 1}` : ""}`}
            value={`${formatDateTime(window.start_at)}-${timeFormatter.format(new Date(window.end_at))}`}
            kind="clock"
          />
        ))}
        {!projection.predicted_sleep_window &&
          !projection.predicted_waking_window &&
          !projection.availability && (
            <article className="message-card minimal">
              <p>Limited profile</p>
              <strong>No schedule windows are shared with this profile.</strong>
            </article>
          )}
      </section>

      <section className="permission-panel" aria-labelledby="permission-heading">
        <div>
          <ShieldGlyph />
          <div>
            <p className="kicker">Visible in this profile</p>
            <h2 id="permission-heading">
              {projection.granted_fields.length} allowlisted{" "}
              {projection.granted_fields.length === 1 ? "field" : "fields"}
            </h2>
          </div>
        </div>
        <ul>
          {projection.granted_fields.map((field) => (
            <li key={field}>{field.replaceAll("_", " ")}</li>
          ))}
        </ul>
      </section>

      <p className="projection-notice">{projection.notice}</p>

      <footer className="view-footer">
        <div>
          <span className="live-dot" aria-hidden="true" />
          <p>
            <strong>Projection current</strong>
            Generated {formatDateTime(projection.generated_at)}
          </p>
        </div>
        <p>Expires {formatDateTime(projection.expires_at)}</p>
      </footer>
    </>
  );
}

function UnavailableProfile() {
  return (
    <section className="status-panel" aria-labelledby="status-heading">
      <div className="phase-dial" aria-hidden="true">
        <span>
          <ShieldGlyph />
        </span>
      </div>
      <div>
        <p className="kicker">Access unavailable</p>
        <h1 id="status-heading">Link unavailable</h1>
        <p>This link is no longer available. Contact the person who shared it directly.</p>
      </div>
    </section>
  );
}

function readInitialProfile() {
  return getFixtureProfile(new URLSearchParams(window.location.search).get("profile"));
}

export default function App() {
  const [profile, setProfile] = useState(readInitialProfile);

  const selectProfile = (nextProfile: FixtureProfile) => {
    setProfile(nextProfile);
    window.history.replaceState(null, "", `?profile=${nextProfile.id}`);
  };

  return (
    <div className="trusted-shell">
      <header className="site-header">
        <a className="trusted-brand" href="?profile=planning" aria-label="Trusted view home">
          <span className="trusted-mark">
            <span />
          </span>
          <span>
            <strong>Non-24</strong>
            <small>Trusted view</small>
          </span>
        </a>
        <div className="secure-label">
          <ShieldGlyph />
          Synthetic private preview
        </div>
      </header>

      <main>
        <nav className="profile-switcher" aria-label="Synthetic sharing profiles">
          <span>Fixture profile</span>
          {profileList.map((item) => (
            <button
              key={item.id}
              type="button"
              aria-pressed={profile.id === item.id}
              onClick={() => selectProfile(item)}
            >
              {item.label}
            </button>
          ))}
        </nav>

        {profile.projection ? <ActiveProfile profile={profile} /> : <UnavailableProfile />}
      </main>

      <p className="privacy-footer">
        <ShieldGlyph />
        Static synthetic prototype - no network access - no private health record
      </p>
    </div>
  );
}
