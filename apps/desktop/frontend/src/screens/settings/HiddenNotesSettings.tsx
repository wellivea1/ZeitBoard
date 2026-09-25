import { noticeCatalog, showAllNotices, showNotice, useHiddenNotices } from "../../state/notices";

// The one place closed notes come back from.
export function HiddenNotesSettings() {
  const hidden = useHiddenNotices();
  return (
    <section className="settings-section" aria-labelledby="hidden-notes-title">
      <div>
        <h2 id="hidden-notes-title">Hidden notes</h2>
        <p className="settings-copy">
          Notes you close with their × are listed here, to show again.
        </p>
      </div>
      {hidden.length === 0 ? (
        <p className="hidden-notes-empty">None are hidden.</p>
      ) : (
        <>
          <ul className="hidden-notes">
            {hidden.map((id) => (
              <li key={id}>
                <span>
                  <strong>{noticeCatalog[id].title}</strong>
                  <small>{noticeCatalog[id].where}</small>
                </span>
                <button className="button ghost" type="button" onClick={() => showNotice(id)}>
                  Show again
                </button>
              </li>
            ))}
          </ul>
          {hidden.length > 1 && (
            <button className="button compact" type="button" onClick={showAllNotices}>
              Show all {hidden.length} again
            </button>
          )}
        </>
      )}
    </section>
  );
}
