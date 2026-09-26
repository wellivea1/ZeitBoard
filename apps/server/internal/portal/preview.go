package portal

import (
	"bytes"
	"fmt"
	"net/http"
	"time"
)

// Preview is what a recipient of one link would see now, rendered by the
// same template from the same projection, for the owner to check before and
// after sharing. It carries no link token and records no access: it is not a
// visit.
type Preview struct {
	// HTML is the visible page, without the document around it. Its only
	// link, "Ask for a time", is inert.
	HTML string
	// Stylesheet is the page's own stylesheet. Its token and page rules also
	// apply under a shadow root (`:host`, `.portal-root`), which is how the
	// owner's app shows the page without framing the public one.
	Stylesheet string
}

// RenderPreview renders the page a recipient of this link would see at now.
// A link that no longer works previews as the page its recipient gets: the
// same empty "unavailable" page, never the last availability it showed.
func RenderPreview(state ProfileState, snapshot Snapshot, now time.Time) (Preview, error) {
	stylesheet, err := assetsFS.ReadFile("assets/portal.css")
	if err != nil {
		return Preview{}, fmt.Errorf("read portal stylesheet: %w", err)
	}
	name, data := "preview-dashboard", pageData{
		Title:      "Availability",
		View:       BuildView(snapshot, now),
		CanRequest: state.Grants.AllowRequests,
		Preview:    true,
	}
	if state.Revoked || state.Expired {
		title, message := genericMessage(http.StatusNotFound)
		name, data = "preview-generic", pageData{Title: title, Error: message, Preview: true}
	}
	var buffer bytes.Buffer
	if err := pageTemplates.ExecuteTemplate(&buffer, name, data); err != nil {
		return Preview{}, fmt.Errorf("render preview: %w", err)
	}
	return Preview{HTML: buffer.String(), Stylesheet: string(stylesheet)}, nil
}
