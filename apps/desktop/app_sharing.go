package main

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

// Owner-side share links (roadmap slice 12a).
//
// P5-a shipped create/list/revoke/erase on the self-hosted instance and nothing
// in the desktop was wired to it, so the Sharing screen said "link creation and
// recipient access are not connected in this build". That was honest about the
// user's experience and wrong about the system's capability, which is the worse
// of the two failures: it described a limitation that had already been lifted.
//
// Like visitor requests, this talks to the backend rather than the local store —
// the portal runs on the instance, and a desktop with sync off has no share
// links to show because there is no portal to hold them.

const (
	// MinShareLinkPasscode mirrors portal.MinPasscodeLength. The desktop cannot
	// import the server module; TestShareLinkLimitsMatchTheServer pins it.
	MinShareLinkPasscode = 6

	// MaxShareLinkDays mirrors portal.MaxLinkLifetime.
	MaxShareLinkDays = 90
)

type ShareGrantsDTO struct {
	WakingWindows bool `json:"wakingWindows"`
	AllowRequests bool `json:"allowRequests"`
	AllowMessages bool `json:"allowMessages"`
}

// ShareAccessDTO is the per-link access audit the instance keeps. It counts
// events; it does not identify who caused them.
type ShareAccessDTO struct {
	Event     string `json:"event"`
	Label     string `json:"label"`
	Count     int    `json:"count"`
	LastLabel string `json:"lastLabel,omitempty"`
}

type ShareLinkDTO struct {
	ProfileID    string           `json:"profileId"`
	Label        string           `json:"label"`
	State        string           `json:"state"`
	StateLabel   string           `json:"stateLabel"`
	CreatedLabel string           `json:"createdLabel"`
	ExpiresLabel string           `json:"expiresLabel"`
	Grants       ShareGrantsDTO   `json:"grants"`
	GrantSummary string           `json:"grantSummary"`
	Access       []ShareAccessDTO `json:"access"`
}

type ShareLinksDTO struct {
	// Status is off, unavailable, ok, or error. `off` and `unavailable` are
	// different facts and the screen says which: sync is not configured, versus
	// the instance is reachable and its portal is switched off.
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`

	// Disclosure is the instance's own wording about what a link reveals. It is
	// shown before a link can be created, never after.
	Disclosure string         `json:"disclosure"`
	Links      []ShareLinkDTO `json:"links"`

	MinPasscodeLength int `json:"minPasscodeLength"`
	MaxDays           int `json:"maxDays"`
}

type CreateShareLinkInput struct {
	Label         string         `json:"label"`
	Passcode      string         `json:"passcode"`
	ExpiresInDays int            `json:"expiresInDays"`
	Grants        ShareGrantsDTO `json:"grants"`
}

// CreatedShareLinkDTO carries the one and only time the link address exists in
// readable form. The instance stores a hash of it, so nothing can show it
// again — the screen has to say that where the owner will read it.
type CreatedShareLinkDTO struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`

	LinkURL      string `json:"linkUrl,omitempty"`
	ExpiresLabel string `json:"expiresLabel,omitempty"`
	Disclosure   string `json:"disclosure,omitempty"`

	Links ShareLinksDTO `json:"links"`
}

type ShareLinkActionInput struct {
	ProfileID string `json:"profileId"`

	// Confirmation is required for erasure and must be the exact profile id.
	// Revocation stops access; erasure removes the record that the link ever
	// existed, and the two must not be one click apart.
	Confirmation string `json:"confirmation"`
}

type backendShareProfileRecord struct {
	ProfileID string    `json:"profileId"`
	Label     string    `json:"label"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Grants    struct {
		WakingWindows bool `json:"wakingWindows"`
		AllowRequests bool `json:"allowRequests"`
		AllowMessages bool `json:"allowMessages"`
	} `json:"grants"`
	Access []struct {
		Event      string    `json:"event"`
		Count      int       `json:"count"`
		LastAccess time.Time `json:"lastAccess"`
	} `json:"access"`
}

// SchemaVersion is declared because the shared client decodes with
// DisallowUnknownFields, so a field present on the wire and absent here is a
// decode failure rather than something ignored. Omitting it made every create
// and every list fail with "invalid JSON response" while both sides' unit tests
// passed, because each was fed its own fixture.
type backendShareProfileListResponse struct {
	SchemaVersion string                      `json:"schema_version"`
	Profiles      []backendShareProfileRecord `json:"profiles"`
	Disclosure    string                      `json:"disclosure"`
}

type backendCreatedShareProfileResponse struct {
	SchemaVersion string    `json:"schema_version"`
	ProfileID     string    `json:"profileId"`
	LinkURL       string    `json:"linkUrl"`
	ExpiresAt     time.Time `json:"expiresAt"`
	Disclosure    string    `json:"disclosure"`
}

// GetBackendShareLinks lists the links this instance holds.
func (a *App) GetBackendShareLinks() (ShareLinksDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return shareLinksUnavailable("off",
			"Sharing runs on your own server. Turn on backend sync in Settings to create a link."), nil
	}
	return a.fetchBackendShareLinks(a.applicationContext(), cfg, token), nil
}

func (a *App) fetchBackendShareLinks(ctx context.Context, cfg backendSyncConfig, token string) ShareLinksDTO {
	client := a.newDesktopBackendClient(cfg, token)
	var response backendShareProfileListResponse
	if err := client.getJSON(ctx, "/v1/portal/profiles", &response); err != nil {
		if isBackendRouteAbsent(err) {
			// The route does not exist at all when `portal.enabled` is false,
			// which is the default and a legitimate configuration.
			return shareLinksUnavailable("unavailable",
				"Your server is reachable, but its portal is switched off. Enable it in the server configuration to share a link.")
		}
		return shareLinksUnavailable("error", sanitizeBackendError(err))
	}

	links := make([]ShareLinkDTO, 0, len(response.Profiles))
	for _, record := range response.Profiles {
		links = append(links, shareLinkDTO(record))
	}
	return ShareLinksDTO{
		Status:            "ok",
		Disclosure:        response.Disclosure,
		Links:             links,
		MinPasscodeLength: MinShareLinkPasscode,
		MaxDays:           MaxShareLinkDays,
	}
}

func shareLinksUnavailable(status, message string) ShareLinksDTO {
	return ShareLinksDTO{
		Status:            status,
		Message:           message,
		Links:             []ShareLinkDTO{},
		MinPasscodeLength: MinShareLinkPasscode,
		MaxDays:           MaxShareLinkDays,
	}
}

func shareLinkDTO(record backendShareProfileRecord) ShareLinkDTO {
	label := strings.TrimSpace(record.Label)
	if label == "" {
		label = "Unnamed link"
	}
	dto := ShareLinkDTO{
		ProfileID:    record.ProfileID,
		Label:        label,
		State:        record.State,
		StateLabel:   shareStateLabel(record.State),
		CreatedLabel: "Created " + record.CreatedAt.Local().Format("Jan 2, 2006"),
		ExpiresLabel: shareExpiryLabel(record.State, record.ExpiresAt),
		Grants: ShareGrantsDTO{
			WakingWindows: record.Grants.WakingWindows,
			AllowRequests: record.Grants.AllowRequests,
			AllowMessages: record.Grants.AllowMessages,
		},
		Access: make([]ShareAccessDTO, 0, len(record.Access)),
	}
	dto.GrantSummary = shareGrantSummary(dto.Grants)
	for _, entry := range record.Access {
		access := ShareAccessDTO{
			Event: entry.Event,
			Label: shareAccessLabel(entry.Event),
			Count: entry.Count,
		}
		if !entry.LastAccess.IsZero() {
			access.LastLabel = "last " + entry.LastAccess.Local().Format("Jan 2, 3:04 PM")
		}
		dto.Access = append(dto.Access, access)
	}
	return dto
}

func shareStateLabel(state string) string {
	switch state {
	case "active":
		return "Working now"
	case "revoked":
		return "Revoked"
	case "expired":
		return "Expired"
	default:
		return state
	}
}

func shareExpiryLabel(state string, expiresAt time.Time) string {
	if expiresAt.IsZero() {
		return ""
	}
	stamp := expiresAt.Local().Format("Jan 2, 2006")
	if state == "active" {
		return "Expires " + stamp
	}
	return "Expired " + stamp
}

// shareGrantSummary states what the link shows in the owner's words rather than
// as a row of flag names.
func shareGrantSummary(grants ShareGrantsDTO) string {
	parts := make([]string, 0, 3)
	if grants.WakingWindows {
		parts = append(parts, "when you are likely awake")
	}
	if grants.AllowRequests {
		parts = append(parts, "can ask for a time")
	}
	if grants.AllowMessages {
		parts = append(parts, "can send a short message")
	}
	if len(parts) == 0 {
		return "Shows nothing yet"
	}
	return "Shows " + strings.Join(parts, "; ")
}

func shareAccessLabel(event string) string {
	switch event {
	case "availability_read":
		return "Opened the page"
	case "passcode_failure":
		return "Wrong passcode"
	case "request_created":
		return "Asked for a time"
	default:
		return event
	}
}

// CreateBackendShareLink mints a link. The address it returns is the only copy:
// the instance keeps a hash so that a stolen database cannot yield working
// links, which also means nothing can show the address a second time.
func (a *App) CreateBackendShareLink(input CreateShareLinkInput) (CreatedShareLinkDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return CreatedShareLinkDTO{
			Status:  "error",
			Message: "Sharing runs on your own server. Turn on backend sync in Settings first.",
			Links: shareLinksUnavailable("off",
				"Sharing runs on your own server. Turn on backend sync in Settings to create a link."),
		}, nil
	}
	if err := validateShareLinkInput(input); err != nil {
		ctx := a.applicationContext()
		return CreatedShareLinkDTO{
			Status:  "error",
			Message: err.Error(),
			Links:   a.fetchBackendShareLinks(ctx, cfg, token),
		}, nil
	}

	ctx := a.applicationContext()
	client := a.newDesktopBackendClient(cfg, token)
	var created backendCreatedShareProfileResponse
	payload := map[string]any{
		"label":         strings.TrimSpace(input.Label),
		"passcode":      input.Passcode,
		"expiresInDays": input.ExpiresInDays,
		"grants": map[string]bool{
			"wakingWindows": input.Grants.WakingWindows,
			"allowRequests": input.Grants.AllowRequests,
			"allowMessages": input.Grants.AllowMessages,
		},
	}
	if err := client.postJSON(ctx, "/v1/portal/profiles", payload, &created); err != nil {
		return CreatedShareLinkDTO{
			Status:  "error",
			Message: sanitizeBackendError(err),
			Links:   a.fetchBackendShareLinks(ctx, cfg, token),
		}, nil
	}
	return CreatedShareLinkDTO{
		Status:       "ok",
		LinkURL:      created.LinkURL,
		ExpiresLabel: "Expires " + created.ExpiresAt.Local().Format("Jan 2, 2006"),
		Disclosure:   created.Disclosure,
		Links:        a.fetchBackendShareLinks(ctx, cfg, token),
	}, nil
}

func validateShareLinkInput(input CreateShareLinkInput) error {
	if strings.TrimSpace(input.Label) == "" {
		return errors.New("Give the link a name so you can tell it apart later. The name stays on this device.")
	}
	if len([]rune(input.Passcode)) < MinShareLinkPasscode {
		return errors.New("A passcode of at least 6 characters is required on every link.")
	}
	if input.ExpiresInDays <= 0 || input.ExpiresInDays > MaxShareLinkDays {
		return errors.New("Choose an expiry between 1 and 90 days. Links do not last forever.")
	}
	if !input.Grants.WakingWindows && !input.Grants.AllowRequests && !input.Grants.AllowMessages {
		return errors.New("A link that shows nothing is not worth sending. Choose at least one thing to share.")
	}
	return nil
}

// RevokeBackendShareLink stops the link working. The record stays so the access
// history remains readable; erasure is a separate, confirmed action.
func (a *App) RevokeBackendShareLink(input ShareLinkActionInput) (ShareLinksDTO, error) {
	return a.shareLinkAction(input.ProfileID, "revoke", "")
}

// EraseBackendShareLink removes the record that the link existed at all. It
// requires the profile id typed back, for the same reason erasing sleep data
// does: a mistaken click must not be able to destroy history.
func (a *App) EraseBackendShareLink(input ShareLinkActionInput) (ShareLinksDTO, error) {
	if strings.TrimSpace(input.Confirmation) != strings.TrimSpace(input.ProfileID) {
		result, _ := a.GetBackendShareLinks()
		result.Status = "error"
		result.Message = "Type the link's id exactly to erase it. Revoking is enough to stop access."
		return result, nil
	}
	return a.shareLinkAction(input.ProfileID, "erase", "")
}

func (a *App) shareLinkAction(profileID, action, _ string) (ShareLinksDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return shareLinksUnavailable("off",
			"Sharing runs on your own server. Turn on backend sync in Settings to manage links."), nil
	}
	if strings.TrimSpace(profileID) == "" {
		result := a.fetchBackendShareLinks(a.applicationContext(), cfg, token)
		result.Status = "error"
		result.Message = "No link was selected."
		return result, nil
	}

	ctx := a.applicationContext()
	client := a.newDesktopBackendClient(cfg, token)
	path := "/v1/portal/profiles/" + url.PathEscape(profileID) + "/" + action
	var response map[string]string
	if err := client.postJSON(ctx, path, map[string]any{}, &response); err != nil {
		result := a.fetchBackendShareLinks(ctx, cfg, token)
		result.Status = "error"
		result.Message = sanitizeBackendError(err)
		return result, nil
	}
	return a.fetchBackendShareLinks(ctx, cfg, token), nil
}
