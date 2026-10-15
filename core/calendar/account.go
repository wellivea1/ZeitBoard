package calendar

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// AccountEvent is one event as a calendar account's API states it (ADR-0064),
// before it is kept. Recurring events arrive already expanded, one per
// occurrence, each with its own provider id. An all-day event is a span of
// dates rather than of instants: its StartAt and EndAt carry its first day
// and the day after its last at midnight UTC, and it is kept in the zone the
// calendar is kept in, as a file's all-day dates are.
type AccountEvent struct {
	ID       string
	Title    string
	Location string
	StartAt  time.Time
	EndAt    time.Time
	ZoneID   string
	AllDay   bool
	Busy     bool
}

// AccountEventSet keeps a calendar account's events as a read-only source,
// held to the same bounds and checks as a parsed file. An untitled event is
// "Busy", as it is in a file, and one outside the coverage is left out. The
// provider names the calendar, so a name too long to keep is shortened.
func AccountEventSet(options ParseOptions, events []AccountEvent) (EventSet, error) {
	options.SourceLabel = truncateRunes(strings.TrimSpace(options.SourceLabel), maxSourceLabelRunes)
	if err := options.validate(); err != nil {
		return EventSet{}, err
	}
	location, calendarZone, err := loadLocation(options.DefaultZoneID)
	if err != nil {
		return EventSet{}, err
	}
	kept := make([]Event, 0, len(events))
	seen := make(map[string]bool, len(events))
	for index, item := range events {
		if strings.TrimSpace(item.ID) == "" {
			return EventSet{}, fmt.Errorf("account event %d has no id", index)
		}
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		start, end, zone := item.StartAt, item.EndAt, item.ZoneID
		if item.AllDay {
			if start, end, err = civilDays(location, item.StartAt, item.EndAt); err != nil {
				return EventSet{}, fmt.Errorf("account event %d: %w", index, err)
			}
			zone = calendarZone
		}
		if zone == "" {
			zone = options.DefaultZoneID
		}
		if !intervalIntersects(start, end, options.CoverageStart, options.CoverageEnd) {
			continue
		}
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = "Busy"
		}
		event := Event{
			EventID:        deterministicEventID(options.SourceID, item.ID, item.StartAt),
			SourceID:       options.SourceID,
			SourceRecordID: truncateRunes(item.ID, maxSourceRecordIDRunes),
			Title:          truncateRunes(title, maxTitleRunes),
			StartAt:        start.UTC(),
			EndAt:          end.UTC(),
			ZoneID:         zone,
			AllDay:         item.AllDay,
			Busy:           item.Busy && start.Before(end),
			Ownership:      OwnershipImported,
			CreatedAt:      options.ImportedAt.UTC(),
			Location:       truncateRunes(strings.TrimSpace(item.Location), maxLocationRunes),
		}
		if err := event.Validate(); err != nil {
			return EventSet{}, fmt.Errorf("account event %d: %w", index, err)
		}
		kept = append(kept, event)
		if len(kept) > MaxMaterializedEvents {
			return EventSet{}, fmt.Errorf("the calendar holds more than %d events", MaxMaterializedEvents)
		}
	}
	sort.Slice(kept, func(i, j int) bool {
		if kept[i].StartAt.Equal(kept[j].StartAt) {
			return kept[i].EventID < kept[j].EventID
		}
		return kept[i].StartAt.Before(kept[j].StartAt)
	})
	return EventSet{
		SchemaVersion: "v1",
		GeneratedAt:   options.ImportedAt.UTC(),
		Sources: []Source{{
			SourceID:        options.SourceID,
			Label:           strings.TrimSpace(options.SourceLabel),
			Kind:            options.Kind,
			ReadOnly:        true,
			CoverageStartAt: options.CoverageStart.UTC(),
			CoverageEndAt:   options.CoverageEnd.UTC(),
			LastImportedAt:  options.ImportedAt.UTC(),
		}},
		Events: kept,
	}, nil
}

// civilDays places an all-day span, whose dates come at midnight UTC, at
// midnight in the calendar's zone.
func civilDays(location *time.Location, start, end time.Time) (time.Time, time.Time, error) {
	place := func(at time.Time) (time.Time, error) {
		year, month, day := at.UTC().Date()
		return resolveCivil(location, year, month, day, 0, 0, 0)
	}
	placedStart, err := place(start)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	placedEnd, err := place(end)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return placedStart, placedEnd, nil
}

// truncateRunes shortens text that would otherwise fail a bound, rather than
// refusing a whole calendar over one long title.
func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit]))
}
