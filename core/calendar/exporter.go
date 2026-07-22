package calendar

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const productID = "-//ZeitBoard//Local Calendar v1//EN"

// ExportOwnedICS returns an RFC 5545 calendar containing only events owned by
// ZeitBoard. Imported events are intentionally omitted even when mixed into
// the input slice.
func ExportOwnedICS(events []Event, generatedAt time.Time) ([]byte, error) {
	if generatedAt.IsZero() {
		return nil, errors.New("calendar export generated_at is required")
	}
	owned := make([]Event, 0, len(events))
	seen := make(map[string]struct{})
	for _, event := range events {
		if event.Ownership != OwnershipAppOwned {
			continue
		}
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("event %q: %w", event.EventID, err)
		}
		if _, found := seen[event.EventID]; found {
			return nil, fmt.Errorf("duplicate app-owned event id %q", event.EventID)
		}
		seen[event.EventID] = struct{}{}
		owned = append(owned, event)
	}
	sort.Slice(owned, func(i, j int) bool {
		if owned[i].StartAt.Equal(owned[j].StartAt) {
			return owned[i].EventID < owned[j].EventID
		}
		return owned[i].StartAt.Before(owned[j].StartAt)
	})

	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:" + productID,
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"X-WR-CALNAME:" + escapeText("ZeitBoard placements"),
	}
	for _, event := range owned {
		startLine, endLine, err := exportInterval(event)
		if err != nil {
			return nil, fmt.Errorf("event %q: %w", event.EventID, err)
		}
		transparency := "OPAQUE"
		if !event.Busy {
			transparency = "TRANSPARENT"
		}
		lines = append(lines,
			"BEGIN:VEVENT",
			"UID:"+event.EventID+"@zeitboard.local",
			"DTSTAMP:"+generatedAt.UTC().Format("20060102T150405Z"),
			"CREATED:"+event.CreatedAt.UTC().Format("20060102T150405Z"),
			startLine,
			endLine,
			"SUMMARY:"+escapeText(event.Title),
			"STATUS:CONFIRMED",
			"TRANSP:"+transparency,
			"X-ZEITBOARD-TASK-ID:"+event.TaskID,
			"X-ZEITBOARD-TASK-REVISION:"+fmt.Sprintf("%d", event.TaskRevision),
			"X-ZEITBOARD-PROPOSAL-ID:"+event.ProposalID,
		)
		if event.Location != "" {
			lines = append(lines, "LOCATION:"+escapeText(event.Location))
		}
		if event.Notes != "" {
			lines = append(lines, "DESCRIPTION:"+escapeText(event.Notes))
		}
		lines = append(lines, "END:VEVENT")
	}
	lines = append(lines, "END:VCALENDAR")

	var output bytes.Buffer
	for _, line := range lines {
		for _, folded := range foldContentLine(line) {
			output.WriteString(folded)
			output.WriteString("\r\n")
		}
	}
	return output.Bytes(), nil
}

func exportInterval(event Event) (string, string, error) {
	if !event.AllDay {
		return "DTSTART:" + event.StartAt.UTC().Format("20060102T150405Z"),
			"DTEND:" + event.EndAt.UTC().Format("20060102T150405Z"), nil
	}
	loc, _, err := loadLocation(event.ZoneID)
	if err != nil {
		return "", "", err
	}
	start := event.StartAt.In(loc)
	end := event.EndAt.In(loc)
	if !isMidnight(start) || !isMidnight(end) || !start.Before(end) {
		return "", "", errors.New("all-day event boundaries must be distinct civil midnights")
	}
	return "DTSTART;VALUE=DATE:" + start.Format("20060102"),
		"DTEND;VALUE=DATE:" + end.Format("20060102"), nil
}

func isMidnight(value time.Time) bool {
	hour, minute, second := value.Clock()
	return hour == 0 && minute == 0 && second == 0 && value.Nanosecond() == 0
}

func escapeText(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\r\n", "\\n")
	value = strings.ReplaceAll(value, "\r", "\\n")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, ";", "\\;")
	return strings.ReplaceAll(value, ",", "\\,")
}

func foldContentLine(value string) []string {
	if len(value) <= 75 {
		return []string{value}
	}
	result := make([]string, 0, len(value)/74+1)
	first := true
	for value != "" {
		limit := 74
		prefix := " "
		if first {
			limit = 75
			prefix = ""
			first = false
		}
		if len(value) <= limit {
			result = append(result, prefix+value)
			break
		}
		end := limit
		for end > 0 && !utf8.RuneStart(value[end]) {
			end--
		}
		if end == 0 {
			end = limit
		}
		result = append(result, prefix+value[:end])
		value = value[end:]
	}
	return result
}
