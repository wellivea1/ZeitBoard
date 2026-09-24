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
  const collisions = scheduled.reduce(
    (total, medication) => total + (medication.schedule?.forecast.collisionCount ?? 0),
    0,
  );

  return (
    // Occasional reading, so it opens on request. The summary still says
    // whether any entered time lands in predicted sleep.
    <details className="medication-feasibility" aria-labelledby="medication-feasibility-title">
      <summary>
        <h2 id="medication-feasibility-title">Schedule feasibility</h2>
        <span>
          {scheduled.length === 0
            ? "No schedules"
            : `${scheduled.length} ${scheduled.length === 1 ? "schedule" : "schedules"}, ${
                collisions === 0 ? "none in predicted sleep" : `${collisions} in predicted sleep`
              } · next 14 days`}
        </span>
      </summary>

      <div className="medication-reminder-state" data-status={reminderStatus}>
        <strong>Reminder delivery: {reminderStatus}</strong>
        <span>{reminderMessage}</span>
      </div>

      {scheduled.length === 0 ? (
        <div className="medication-feasibility-empty">
          <strong>No medication schedules stored</strong>
          <p>Use Schedule on a medication to compare the times you entered with predicted sleep.</p>
        </div>
      ) : (
        <div className="medication-feasibility-list">
          {scheduled.map((medication) => {
            const schedule = medication.schedule!;
            const forecast = schedule.forecast;
            // Only a clock change makes this column say anything.
            const clockChange = forecast.occurrences.some((occurrence) => occurrence.dstNote);
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
                          <th scope="col">Scheduled</th>
                          <th scope="col">Predicted rhythm</th>
                          <th scope="col">Confidence</th>
                          {clockChange && <th scope="col">Clock change</th>}
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
                            {clockChange && <td>{occurrence.dstNote ?? "No clock change"}</td>}
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
    </details>
  );
}
