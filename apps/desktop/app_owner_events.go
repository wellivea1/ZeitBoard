package main

import (
	"errors"
	"strings"
	"time"

	"non24.app/core/ownerevents"
	storage "non24.app/core/storage/sqlite"
)

// Events the owner adds (ADR-0068): added, changed and deleted here, kept as
// fixed commitments the planner works around, and synced as revisions to the
// owner's server and on to every device.

// OwnerEventInput is an event as the owner entered it. A new event has no id
// and no revision; a change names the revision the owner was looking at.
type OwnerEventInput struct {
	EventID  string `json:"eventId"`
	Revision int    `json:"revision"`
	Title    string `json:"title"`
	// StartLocal and EndLocal are RFC 3339 with the offset of the occurrence
	// meant, or a local date and time in ZoneID.
	StartLocal string `json:"startLocal"`
	EndLocal   string `json:"endLocal"`
	ZoneID     string `json:"zoneId"`
}

// OwnerEventRefInput names an event, and for a deletion the revision the
// owner was looking at.
type OwnerEventRefInput struct {
	EventID  string `json:"eventId"`
	Revision int    `json:"revision"`
}

type OwnerEventDTO struct {
	EventID    string `json:"eventId"`
	Revision   int    `json:"revision"`
	Title      string `json:"title"`
	StartLocal string `json:"startLocal"`
	EndLocal   string `json:"endLocal"`
	ZoneID     string `json:"zoneId"`
	StartLabel string `json:"startLabel"`
	EndLabel   string `json:"endLabel"`
}

func (a *App) AddOwnerEvent(input OwnerEventInput) (OwnerEventDTO, error) {
	now := a.currentTime().UTC()
	record, err := ownerEventFromInput(input, newLocalID("event"), 1, now)
	if err != nil {
		return OwnerEventDTO{}, err
	}
	record.CreatedAt = now
	return a.keepOwnerEvent(record, func(store *storage.Store) error {
		return store.AddOwnerEvent(a.applicationContext(), record)
	})
}

func (a *App) ChangeOwnerEvent(input OwnerEventInput) (OwnerEventDTO, error) {
	if input.Revision < 1 {
		return OwnerEventDTO{}, errors.New("Open the event again before changing it.")
	}
	now := a.currentTime().UTC()
	record, err := ownerEventFromInput(input, input.EventID, input.Revision+1, now)
	if err != nil {
		return OwnerEventDTO{}, err
	}
	// The store keeps the event's own creation time; this one only passes the
	// check that an event is created before it is changed.
	record.CreatedAt = now
	return a.keepOwnerEvent(record, func(store *storage.Store) error {
		return store.ChangeOwnerEvent(a.applicationContext(), record)
	})
}

func (a *App) DeleteOwnerEvent(input OwnerEventRefInput) error {
	store, err := a.requireStore()
	if err != nil {
		return err
	}
	if err := store.DeleteOwnerEvent(a.applicationContext(), input.EventID, input.Revision); err != nil {
		return ownerEventProblem(err)
	}
	a.wakeDesktopBackground()
	return nil
}

func (a *App) GetOwnerEvent(input OwnerEventRefInput) (OwnerEventDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return OwnerEventDTO{}, err
	}
	record, err := store.OwnerEvent(a.applicationContext(), input.EventID)
	if err != nil {
		return OwnerEventDTO{}, ownerEventProblem(err)
	}
	return ownerEventDTO(record), nil
}

// keepOwnerEvent saves an event, wakes the sync that carries it to the
// owner's server, and gives the event as it now stands.
func (a *App) keepOwnerEvent(record ownerevents.Record, save func(*storage.Store) error) (OwnerEventDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return OwnerEventDTO{}, err
	}
	if err := save(store); err != nil {
		return OwnerEventDTO{}, ownerEventProblem(err)
	}
	a.wakeDesktopBackground()
	saved, err := store.OwnerEvent(a.applicationContext(), record.EventID)
	if err != nil {
		return OwnerEventDTO{}, ownerEventProblem(err)
	}
	return ownerEventDTO(saved), nil
}

// ownerEventFromInput reads an entered event as the revision it would be.
func ownerEventFromInput(input OwnerEventInput, eventID string, revision int, now time.Time) (ownerevents.Record, error) {
	title := strings.TrimSpace(input.Title)
	if err := ownerevents.ValidateTitle(title); err != nil {
		return ownerevents.Record{}, errors.New("Give the event a title of 1 to 120 characters, on one line.")
	}
	zoneID := strings.TrimSpace(input.ZoneID)
	if zoneID == "" {
		zoneID = localZoneID()
	}
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		return ownerevents.Record{}, errors.New("ZeitBoard does not know that time zone.")
	}
	start, err := parseCivilTime(strings.TrimSpace(input.StartLocal), location)
	if err != nil {
		return ownerevents.Record{}, errors.New("The start time cannot be read.")
	}
	end, err := parseCivilTime(strings.TrimSpace(input.EndLocal), location)
	if err != nil {
		return ownerevents.Record{}, errors.New("The end time cannot be read.")
	}
	record := ownerevents.Record{
		EventID: eventID, Title: title, StartAt: start.UTC(), EndAt: end.UTC(), ZoneID: zoneID,
		Revision: revision, CreatedAt: now, UpdatedAt: now,
	}
	switch {
	case !record.EndAt.After(record.StartAt):
		return ownerevents.Record{}, errors.New("An event must end after it starts.")
	case record.EndAt.Sub(record.StartAt) > ownerevents.MaxLength:
		return ownerevents.Record{}, errors.New("An event can last at most a day.")
	}
	return record, nil
}

// ownerEventProblem says what went wrong as a sentence.
func ownerEventProblem(err error) error {
	switch {
	case errors.Is(err, storage.ErrOwnerEventNotFound):
		return errors.New("That event is no longer here.")
	case errors.Is(err, storage.ErrOwnerEventChanged):
		return errors.New("That event changed meanwhile. Open it again before changing it.")
	default:
		return asSentence(err)
	}
}

func ownerEventDTO(record ownerevents.Record) OwnerEventDTO {
	return OwnerEventDTO{
		EventID:    record.EventID,
		Revision:   record.Revision,
		Title:      record.Title,
		StartLocal: zonedInputValue(record.StartAt, record.ZoneID),
		EndLocal:   zonedInputValue(record.EndAt, record.ZoneID),
		ZoneID:     record.ZoneID,
		StartLabel: calendarTimeLabel(record.StartAt, record.ZoneID, false),
		EndLabel:   calendarTimeLabel(record.EndAt, record.ZoneID, false),
	}
}

// zonedInputValue is an instant as RFC 3339 in its own zone, as the window's
// forms read a stored time.
func zonedInputValue(value time.Time, zoneID string) string {
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		location = time.UTC
	}
	return value.In(location).Format(time.RFC3339Nano)
}
