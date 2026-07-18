import { PageHeader, PlaceholderNotice } from "../components/AppShell";

const days = ["Mon 15", "Tue 16", "Wed 17", "Thu 18", "Fri 19"];

export function CalendarScreen() {
  return (
    <>
      <PageHeader
        title="Calendar"
        description="Compare fixed events with uncertain predicted sleep and waking windows."
        actions={
          <div className="status-cluster">
            <span className="sync-dot" data-mode="fixture" aria-hidden="true" />
            <span>Sample preview</span>
          </div>
        }
      />
      <PlaceholderNotice>
        This five-day board is a synthetic design preview. Calendar import arrives with the
        interoperability phase; fixed events will remain immutable inputs.
      </PlaceholderNotice>
      <section className="panel calendar-board" aria-label="Five day planning preview">
        <div className="calendar-hours" aria-hidden="true">
          <span>12 AM</span>
          <span>6 AM</span>
          <span>12 PM</span>
          <span>6 PM</span>
          <span>12 AM</span>
        </div>
        <div className="calendar-days">
          {days.map((day, index) => (
            <article key={day}>
              <h2>{day}</h2>
              <div className="day-track">
                <span className="sleep-band" style={{ left: `${4 + index * 6}%`, width: "25%" }}>
                  Predicted sleep
                </span>
                <span className="wake-band" style={{ left: `${34 + index * 6}%`, width: "39%" }}>
                  Useful window
                </span>
                {index === 0 && (
                  <span className="fixed-event" style={{ left: "75%" }}>
                    Check-in
                  </span>
                )}
              </div>
            </article>
          ))}
        </div>
      </section>
    </>
  );
}
