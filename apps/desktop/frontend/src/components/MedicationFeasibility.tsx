import type {
  MedicationDefinition,
  MedicationForecastStatus,
  MedicationReminderStatus,
} from "../data/medications";

function forecastStatusLabel(status: MedicationForecastStatus): string {
  switch (status) {
    case "collision":
      return "Current overlap present";
    case "no_overlap":
      return "No current overlap";
    case "not_applicable":
      return "No timed forecast";
    default:
      return "Forecast coverage unavailable";
  }
}

function reminderLabel(medication: MedicationDefinition): string {
  if (!medication.active) return "Paused while archived";
  return medication.schedule?.reminderEnabled ? "Desktop reminder on" : "Desktop reminder off";
}

export function MedicationFeasibility({
  medications,
  reminderStatus,
  reminderMessage,
}: {
  medications: MedicationDefinition[];
  reminderStatus: MedicationReminderStatus;
  reminderMessage: string;
}) {
  const scheduled = medications.filter((medication) => medication.schedule !== undefined);

  return (
    <section className="medication-feasibility" aria-labelledby="medication-feasibility-title">
      <header className="medication-section-heading">
        <div>
          <p className="section-kicker">User-authored timing</p>
          <h2 id="medication-feasibility-title">Schedule feasibility</h2>
        </div>
        <span>Next 14 civil days</span>
      </header>

      <div className="medication-reminder-state" data-status={reminderStatus}>
        <strong>Reminder delivery: {reminderStatus}</strong>
        <span>{reminderMessage}</span>
      </div>

      {scheduled.length === 0 ? (
        <div className="medication-feasibility-empty">
          <strong>No medication schedules stored</strong>
          <p>
            Add a schedule from the medication rail to compare only the times you entered with the
            current rhythm forecast.
          </p>
        </div>
      ) : (
        <div className="medication-feasibility-list">
          {scheduled.map((medication) => {
            const schedule = medication.schedule!;
            const forecast = schedule.forecast;
            return (
              <article key={medication.medicationId} data-forecast={forecast.status}>
                <header className="medication-feasibility-record-header">
                  <div>
                    <strong>{medication.label}</strong>
                    <span>{schedule.summary}</span>
                  </div>
                  <div>
                    <span>{forecastStatusLabel(forecast.status)}</span>
                    <small>{reminderLabel(medication)}</small>
                  </div>
                </header>

                {medication.clinicianRule && medication.clinicianRuleAttribution && (
                  <div className="medication-clinician-rule">
                    <span>{medication.clinicianRuleAttribution}</span>
                    <blockquote>{medication.clinicianRule}</blockquote>
                  </div>
                )}

                <div className="medication-forecast-summary">
                  <p>{forecast.message}</p>
                  <dl>
                    <div>
                      <dt>Covered</dt>
                      <dd>{forecast.coveredCount}</dd>
                    </div>
                    <div>
                      <dt>Inside predicted sleep</dt>
                      <dd>{forecast.collisionCount}</dd>
                    </div>
                    <div>
                      <dt>Outside horizon</dt>
                      <dd>{forecast.outsideHorizonCount}</dd>
                    </div>
                    <div>
                      <dt>Coverage ends</dt>
                      <dd>{forecast.coverageLabel ?? "Not available"}</dd>
                    </div>
                  </dl>
                </div>

                {forecast.occurrences.length > 0 && (
                  <div
                    className="medication-forecast-table-wrap"
                    role="region"
                    aria-label={`${medication.label} schedule occurrences`}
                    tabIndex={0}
                  >
                    <table className="medication-forecast-table">
                      <thead>
                        <tr>
                          <th scope="col">Scheduled civil time</th>
                          <th scope="col">Forecast context</th>
                          <th scope="col">Confidence</th>
                          <th scope="col">DST handling</th>
                        </tr>
                      </thead>
                      <tbody>
                        {forecast.occurrences.map((occurrence) => (
                          <tr
                            key={`${occurrence.at}-${occurrence.civilTime}`}
                            data-status={occurrence.status}
                          >
                            <td>
                              <time dateTime={occurrence.at}>{occurrence.civilLabel}</time>
                            </td>
                            <td>{occurrence.context}</td>
                            <td>{occurrence.confidence}</td>
                            <td>{occurrence.dstNote ?? "Standard civil-time occurrence"}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}

                {forecast.gaps.length > 0 && (
                  <div className="medication-schedule-gaps" aria-label="DST schedule gaps">
                    {forecast.gaps.map((gap) => (
                      <div key={`${gap.civilDate}-${gap.civilTime}`}>
                        <strong>{gap.civilLabel}</strong>
                        <span>{gap.message}</span>
                      </div>
                    ))}
                  </div>
                )}
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
}
