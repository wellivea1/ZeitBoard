package calendar

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	rrule "github.com/teambition/rrule-go"
)

const (
	maxUnfoldedLineBytes = 1 << 20
	maxRecurrenceWork    = 250_000
	maxEventDays         = 366
)

var durationPattern = regexp.MustCompile(`^P(?:(\d+)W|(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?)$`)

type contentLine struct {
	name   string
	params map[string][]string
	value  string
	line   int
}

type component struct {
	lines []contentLine
	index int
}

type temporal struct {
	value  time.Time
	zoneID string
	allDay bool
}

type eventDuration struct {
	days  int
	clock time.Duration
}

type parsedEvent struct {
	uid             string
	recurrenceID    *temporal
	start           temporal
	duration        eventDuration
	hasDuration     bool
	title           string
	location        string
	notes           string
	busy            bool
	cancelled       bool
	rrule           string
	rdates          []time.Time
	exdates         []time.Time
	componentNumber int
}

func ParseICS(data []byte, options ParseOptions) (EventSet, error) {
	if err := options.validate(); err != nil {
		return EventSet{}, err
	}
	if len(data) == 0 {
		return EventSet{}, errors.New("calendar document is empty")
	}
	if len(data) > MaxDocumentBytes {
		return EventSet{}, fmt.Errorf("calendar document exceeds %d bytes", MaxDocumentBytes)
	}
	if !utf8.Valid(data) {
		return EventSet{}, errors.New("calendar document is not valid UTF-8")
	}

	lines, err := unfoldContentLines(string(data))
	if err != nil {
		return EventSet{}, err
	}
	components, err := collectEventComponents(lines)
	if err != nil {
		return EventSet{}, err
	}
	if len(components) > MaxEventComponents {
		return EventSet{}, fmt.Errorf("calendar contains more than %d VEVENT components", MaxEventComponents)
	}

	parsed := make([]parsedEvent, 0, len(components))
	for _, item := range components {
		event, err := parseEventComponent(item, options.DefaultZoneID)
		if err != nil {
			return EventSet{}, fmt.Errorf("VEVENT %d: %w", item.index, err)
		}
		parsed = append(parsed, event)
	}

	events, err := materializeEvents(parsed, options)
	if err != nil {
		return EventSet{}, err
	}
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
		Events: events,
	}, nil
}

func unfoldContentLines(document string) ([]contentLine, error) {
	document = strings.TrimPrefix(document, "\ufeff")
	document = strings.ReplaceAll(document, "\r\n", "\n")
	if strings.ContainsRune(document, '\r') {
		return nil, errors.New("calendar contains a bare carriage return")
	}

	physical := strings.Split(document, "\n")
	unfolded := make([]struct {
		value string
		line  int
	}, 0, len(physical))
	for index, line := range physical {
		lineNumber := index + 1
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if len(unfolded) == 0 {
				return nil, fmt.Errorf("line %d: folded content has no preceding line", lineNumber)
			}
			last := &unfolded[len(unfolded)-1]
			last.value += line[1:]
			if len(last.value) > maxUnfoldedLineBytes {
				return nil, fmt.Errorf("line %d: unfolded content line exceeds %d bytes", last.line, maxUnfoldedLineBytes)
			}
			continue
		}
		if len(line) > maxUnfoldedLineBytes {
			return nil, fmt.Errorf("line %d: content line exceeds %d bytes", lineNumber, maxUnfoldedLineBytes)
		}
		unfolded = append(unfolded, struct {
			value string
			line  int
		}{line, lineNumber})
	}

	result := make([]contentLine, 0, len(unfolded))
	for _, item := range unfolded {
		line, err := parseContentLine(item.value, item.line)
		if err != nil {
			return nil, err
		}
		result = append(result, line)
	}
	return result, nil
}

func parseContentLine(raw string, lineNumber int) (contentLine, error) {
	colon := delimiterOutsideQuotes(raw, ':')
	if colon < 1 {
		return contentLine{}, fmt.Errorf("line %d: invalid iCalendar content line", lineNumber)
	}
	left := raw[:colon]
	value := raw[colon+1:]
	parts, err := splitOutsideQuotes(left, ';')
	if err != nil {
		return contentLine{}, fmt.Errorf("line %d: %w", lineNumber, err)
	}
	name := strings.ToUpper(strings.TrimSpace(parts[0]))
	if name == "" {
		return contentLine{}, fmt.Errorf("line %d: property name is empty", lineNumber)
	}
	params := make(map[string][]string, len(parts)-1)
	for _, part := range parts[1:] {
		equals := delimiterOutsideQuotes(part, '=')
		if equals < 1 || equals == len(part)-1 {
			return contentLine{}, fmt.Errorf("line %d: invalid parameter %q", lineNumber, part)
		}
		key := strings.ToUpper(strings.TrimSpace(part[:equals]))
		values, err := splitOutsideQuotes(part[equals+1:], ',')
		if err != nil {
			return contentLine{}, fmt.Errorf("line %d: parameter %s: %w", lineNumber, key, err)
		}
		for _, candidate := range values {
			decoded, err := decodeParameter(candidate)
			if err != nil {
				return contentLine{}, fmt.Errorf("line %d: parameter %s: %w", lineNumber, key, err)
			}
			params[key] = append(params[key], decoded)
		}
	}
	return contentLine{name: name, params: params, value: value, line: lineNumber}, nil
}

func delimiterOutsideQuotes(value string, delimiter byte) int {
	quoted := false
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '"':
			quoted = !quoted
		case delimiter:
			if !quoted {
				return index
			}
		}
	}
	return -1
}

func splitOutsideQuotes(value string, delimiter byte) ([]string, error) {
	var result []string
	start := 0
	quoted := false
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '"':
			quoted = !quoted
		case delimiter:
			if !quoted {
				result = append(result, value[start:index])
				start = index + 1
			}
		}
	}
	if quoted {
		return nil, errors.New("unterminated quoted parameter")
	}
	result = append(result, value[start:])
	return result, nil
}

func decodeParameter(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, `"`) {
		if len(value) < 2 || !strings.HasSuffix(value, `"`) {
			return "", errors.New("unterminated quoted parameter")
		}
		value = value[1 : len(value)-1]
	}
	var result strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '^' {
			result.WriteByte(value[index])
			continue
		}
		if index+1 >= len(value) {
			return "", errors.New("dangling RFC 6868 escape")
		}
		index++
		switch value[index] {
		case '^':
			result.WriteByte('^')
		case 'n', 'N':
			result.WriteByte('\n')
		case '\'':
			result.WriteByte('"')
		default:
			return "", fmt.Errorf("unknown RFC 6868 escape ^%c", value[index])
		}
	}
	return result.String(), nil
}

func collectEventComponents(lines []contentLine) ([]component, error) {
	stack := make([]string, 0, 4)
	components := make([]component, 0)
	var current *component
	seenCalendar := false
	closedCalendar := false
	versionSeen := false

	for _, line := range lines {
		switch line.name {
		case "BEGIN":
			name := strings.ToUpper(strings.TrimSpace(line.value))
			if name == "" {
				return nil, fmt.Errorf("line %d: BEGIN component is empty", line.line)
			}
			if len(stack) == 0 {
				if name != "VCALENDAR" || seenCalendar || closedCalendar {
					return nil, fmt.Errorf("line %d: expected one VCALENDAR root", line.line)
				}
				seenCalendar = true
			}
			if name == "VEVENT" {
				if len(stack) == 0 || stack[len(stack)-1] != "VCALENDAR" || current != nil {
					return nil, fmt.Errorf("line %d: VEVENT must be a direct VCALENDAR child", line.line)
				}
				current = &component{index: len(components) + 1}
			}
			stack = append(stack, name)
		case "END":
			name := strings.ToUpper(strings.TrimSpace(line.value))
			if len(stack) == 0 || stack[len(stack)-1] != name {
				return nil, fmt.Errorf("line %d: END:%s does not match the open component", line.line, name)
			}
			if name == "VEVENT" {
				components = append(components, *current)
				current = nil
			}
			stack = stack[:len(stack)-1]
			if name == "VCALENDAR" {
				closedCalendar = true
			}
		default:
			if len(stack) == 0 {
				return nil, fmt.Errorf("line %d: property appears outside VCALENDAR", line.line)
			}
			if len(stack) == 1 && stack[0] == "VCALENDAR" && line.name == "VERSION" {
				if versionSeen {
					return nil, fmt.Errorf("line %d: VERSION appears more than once", line.line)
				}
				if strings.TrimSpace(line.value) != "2.0" {
					return nil, fmt.Errorf("line %d: unsupported iCalendar VERSION %q", line.line, line.value)
				}
				versionSeen = true
			}
			if current != nil && stack[len(stack)-1] == "VEVENT" {
				current.lines = append(current.lines, line)
			}
		}
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("calendar ended with unclosed %s component", stack[len(stack)-1])
	}
	if !seenCalendar || !closedCalendar {
		return nil, errors.New("calendar is missing a complete VCALENDAR root")
	}
	if !versionSeen {
		return nil, errors.New("calendar is missing VERSION:2.0")
	}
	return components, nil
}

func parseEventComponent(item component, defaultZoneID string) (parsedEvent, error) {
	uidLine, err := exactlyOne(item.lines, "UID", true)
	if err != nil {
		return parsedEvent{}, err
	}
	uid := strings.TrimSpace(uidLine.value)
	if err := boundedText("UID", uid, 1, 400); err != nil {
		return parsedEvent{}, err
	}
	if strings.ContainsAny(uid, "\r\n\x00") {
		return parsedEvent{}, errors.New("UID contains a control character")
	}

	startLine, err := exactlyOne(item.lines, "DTSTART", true)
	if err != nil {
		return parsedEvent{}, err
	}
	start, err := parseTemporal(startLine, defaultZoneID)
	if err != nil {
		return parsedEvent{}, fmt.Errorf("DTSTART: %w", err)
	}

	event := parsedEvent{
		uid:             uid,
		start:           start,
		title:           "Busy",
		busy:            true,
		componentNumber: item.index,
	}
	if line, err := exactlyOne(item.lines, "SUMMARY", false); err != nil {
		return parsedEvent{}, err
	} else if line.name != "" {
		event.title, err = decodeText(line.value)
		if err != nil {
			return parsedEvent{}, fmt.Errorf("SUMMARY: %w", err)
		}
		event.title = strings.TrimSpace(event.title)
		if event.title == "" {
			event.title = "Busy"
		}
	}
	if err := boundedText("event title", event.title, 1, maxTitleRunes); err != nil {
		return parsedEvent{}, err
	}
	if line, err := exactlyOne(item.lines, "LOCATION", false); err != nil {
		return parsedEvent{}, err
	} else if line.name != "" {
		event.location, err = decodeText(line.value)
		if err != nil {
			return parsedEvent{}, fmt.Errorf("LOCATION: %w", err)
		}
		event.location = strings.TrimSpace(event.location)
		if event.location != "" {
			if err := boundedText("event location", event.location, 1, maxLocationRunes); err != nil {
				return parsedEvent{}, err
			}
		}
	}
	if line, err := exactlyOne(item.lines, "DESCRIPTION", false); err != nil {
		return parsedEvent{}, err
	} else if line.name != "" {
		event.notes, err = decodeText(line.value)
		if err != nil {
			return parsedEvent{}, fmt.Errorf("DESCRIPTION: %w", err)
		}
		event.notes = strings.TrimSpace(event.notes)
		if event.notes != "" {
			if err := boundedText("event notes", event.notes, 1, maxNotesRunes); err != nil {
				return parsedEvent{}, err
			}
		}
	}

	if line, err := exactlyOne(item.lines, "STATUS", false); err != nil {
		return parsedEvent{}, err
	} else if line.name != "" {
		switch strings.ToUpper(strings.TrimSpace(line.value)) {
		case "CANCELLED":
			event.cancelled = true
			event.busy = false
		case "CONFIRMED", "TENTATIVE":
		default:
			return parsedEvent{}, fmt.Errorf("unsupported STATUS %q", line.value)
		}
	}
	if line, err := exactlyOne(item.lines, "TRANSP", false); err != nil {
		return parsedEvent{}, err
	} else if line.name != "" {
		switch strings.ToUpper(strings.TrimSpace(line.value)) {
		case "TRANSPARENT":
			event.busy = false
		case "OPAQUE":
		default:
			return parsedEvent{}, fmt.Errorf("unsupported TRANSP %q", line.value)
		}
	}

	endLine, err := exactlyOne(item.lines, "DTEND", false)
	if err != nil {
		return parsedEvent{}, err
	}
	durationLine, err := exactlyOne(item.lines, "DURATION", false)
	if err != nil {
		return parsedEvent{}, err
	}
	if endLine.name != "" && durationLine.name != "" {
		return parsedEvent{}, errors.New("DTEND and DURATION are mutually exclusive")
	}
	switch {
	case endLine.name != "":
		end, err := parseTemporal(endLine, defaultZoneID)
		if err != nil {
			return parsedEvent{}, fmt.Errorf("DTEND: %w", err)
		}
		if start.allDay != end.allDay {
			return parsedEvent{}, errors.New("DTSTART and DTEND must use the same value type")
		}
		if end.value.Before(start.value) {
			return parsedEvent{}, errors.New("DTEND precedes DTSTART")
		}
		if start.allDay {
			days, err := civilDayDifference(start.value, end.value)
			if err != nil {
				return parsedEvent{}, err
			}
			event.duration.days = days
		} else {
			event.duration.clock = end.value.Sub(start.value)
		}
		event.hasDuration = true
	case durationLine.name != "":
		event.duration, err = parseICalDuration(durationLine.value)
		if err != nil {
			return parsedEvent{}, fmt.Errorf("DURATION: %w", err)
		}
		event.hasDuration = true
	case start.allDay:
		event.duration.days = 1
		event.hasDuration = true
	default:
		event.busy = false
	}
	if eventDurationLimit(event.duration) > maxEventDays*24*time.Hour {
		return parsedEvent{}, fmt.Errorf("event duration exceeds %d days", maxEventDays)
	}
	if event.busy && event.duration.days == 0 && event.duration.clock == 0 {
		event.busy = false
	}

	if line, err := exactlyOne(item.lines, "RECURRENCE-ID", false); err != nil {
		return parsedEvent{}, err
	} else if line.name != "" {
		if values := line.params["RANGE"]; len(values) != 0 {
			return parsedEvent{}, errors.New("RECURRENCE-ID RANGE is not supported")
		}
		value, err := parseTemporal(line, defaultZoneID)
		if err != nil {
			return parsedEvent{}, fmt.Errorf("RECURRENCE-ID: %w", err)
		}
		event.recurrenceID = &value
	}
	if line, err := exactlyOne(item.lines, "RRULE", false); err != nil {
		return parsedEvent{}, err
	} else if line.name != "" {
		event.rrule = strings.TrimSpace(line.value)
	}
	if event.recurrenceID != nil && event.rrule != "" {
		return parsedEvent{}, errors.New("a recurrence exception cannot define RRULE")
	}

	for _, line := range matching(item.lines, "RDATE") {
		values, err := parseTemporalList(line, defaultZoneID, start.allDay)
		if err != nil {
			return parsedEvent{}, fmt.Errorf("RDATE: %w", err)
		}
		event.rdates = append(event.rdates, values...)
	}
	for _, line := range matching(item.lines, "EXDATE") {
		values, err := parseTemporalList(line, defaultZoneID, start.allDay)
		if err != nil {
			return parsedEvent{}, fmt.Errorf("EXDATE: %w", err)
		}
		event.exdates = append(event.exdates, values...)
	}
	if event.recurrenceID != nil && (len(event.rdates) != 0 || len(event.exdates) != 0) {
		return parsedEvent{}, errors.New("a recurrence exception cannot define RDATE or EXDATE")
	}
	return event, nil
}

func exactlyOne(lines []contentLine, name string, required bool) (contentLine, error) {
	matches := matching(lines, name)
	if len(matches) > 1 {
		return contentLine{}, fmt.Errorf("%s appears more than once", name)
	}
	if len(matches) == 0 {
		if required {
			return contentLine{}, fmt.Errorf("%s is required", name)
		}
		return contentLine{}, nil
	}
	return matches[0], nil
}

func matching(lines []contentLine, name string) []contentLine {
	var result []contentLine
	for _, line := range lines {
		if line.name == name {
			result = append(result, line)
		}
	}
	return result
}

func parseTemporalList(line contentLine, defaultZoneID string, allDay bool) ([]time.Time, error) {
	if values := line.params["VALUE"]; len(values) == 1 && strings.EqualFold(values[0], "PERIOD") {
		return nil, errors.New("PERIOD recurrence values are not supported")
	}
	parts := strings.Split(line.value, ",")
	if len(parts) == 0 {
		return nil, errors.New("recurrence date list is empty")
	}
	result := make([]time.Time, 0, len(parts))
	for _, part := range parts {
		copyLine := line
		copyLine.value = part
		parsed, err := parseTemporal(copyLine, defaultZoneID)
		if err != nil {
			return nil, err
		}
		if parsed.allDay != allDay {
			return nil, errors.New("recurrence dates must match DTSTART value type")
		}
		result = append(result, parsed.value)
	}
	return result, nil
}

func parseTemporal(line contentLine, defaultZoneID string) (temporal, error) {
	valueKind, err := singleParameter(line, "VALUE")
	if err != nil {
		return temporal{}, err
	}
	tzid, err := singleParameter(line, "TZID")
	if err != nil {
		return temporal{}, err
	}
	raw := strings.TrimSpace(line.value)
	allDay := strings.EqualFold(valueKind, "DATE") || (valueKind == "" && len(raw) == 8)
	if valueKind != "" && !strings.EqualFold(valueKind, "DATE") && !strings.EqualFold(valueKind, "DATE-TIME") {
		return temporal{}, fmt.Errorf("unsupported VALUE %q", valueKind)
	}
	if allDay {
		if tzid != "" || strings.HasSuffix(raw, "Z") {
			return temporal{}, errors.New("DATE value cannot carry TZID or UTC suffix")
		}
		loc, canonical, err := loadLocation(defaultZoneID)
		if err != nil {
			return temporal{}, err
		}
		year, month, day, _, _, _, err := parseBasicDateTime(raw, true)
		if err != nil {
			return temporal{}, err
		}
		resolved, err := resolveCivil(loc, year, month, day, 0, 0, 0)
		if err != nil {
			return temporal{}, err
		}
		return temporal{value: resolved, zoneID: canonical, allDay: true}, nil
	}

	if strings.HasSuffix(raw, "Z") {
		if tzid != "" {
			return temporal{}, errors.New("UTC DATE-TIME cannot also carry TZID")
		}
		value, err := parseUTCDateTime(raw)
		return temporal{value: value, zoneID: "UTC"}, err
	}
	zone := tzid
	if zone == "" {
		zone = defaultZoneID
	}
	loc, canonical, err := loadLocation(zone)
	if err != nil {
		return temporal{}, err
	}
	year, month, day, hour, minute, second, err := parseBasicDateTime(raw, false)
	if err != nil {
		return temporal{}, err
	}
	resolved, err := resolveCivil(loc, year, month, day, hour, minute, second)
	if err != nil {
		return temporal{}, err
	}
	return temporal{value: resolved, zoneID: canonical}, nil
}

func singleParameter(line contentLine, name string) (string, error) {
	values := line.params[name]
	if len(values) > 1 {
		return "", fmt.Errorf("%s parameter appears more than once", name)
	}
	if len(values) == 0 {
		return "", nil
	}
	return values[0], nil
}

func parseUTCDateTime(raw string) (time.Time, error) {
	for _, layout := range []string{"20060102T150405Z", "20060102T1504Z"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid UTC DATE-TIME %q", raw)
}

func parseBasicDateTime(raw string, dateOnly bool) (int, time.Month, int, int, int, int, error) {
	layouts := []string{"20060102T150405", "20060102T1504"}
	if dateOnly {
		layouts = []string{"20060102"}
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, raw, time.UTC); err == nil {
			year, month, day := parsed.Date()
			hour, minute, second := parsed.Clock()
			return year, month, day, hour, minute, second, nil
		}
	}
	return 0, 0, 0, 0, 0, 0, fmt.Errorf("invalid iCalendar date/time %q", raw)
}

func decodeText(value string) (string, error) {
	var result strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '\\' {
			result.WriteByte(value[index])
			continue
		}
		if index+1 >= len(value) {
			return "", errors.New("dangling text escape")
		}
		index++
		switch value[index] {
		case 'n', 'N':
			result.WriteByte('\n')
		case '\\', ';', ',':
			result.WriteByte(value[index])
		default:
			return "", fmt.Errorf("unknown text escape \\%c", value[index])
		}
	}
	return result.String(), nil
}

func parseICalDuration(raw string) (eventDuration, error) {
	raw = strings.TrimSpace(strings.ToUpper(raw))
	match := durationPattern.FindStringSubmatch(raw)
	if match == nil || raw == "P" || raw == "PT" {
		return eventDuration{}, fmt.Errorf("invalid positive duration %q", raw)
	}
	values := make([]int, 5)
	for index, value := range match[1:] {
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return eventDuration{}, err
		}
		values[index] = parsed
	}
	if strings.Contains(raw, "T") && values[2] == 0 && values[3] == 0 && values[4] == 0 {
		return eventDuration{}, errors.New("duration time designator has no time value")
	}
	if values[0] > maxEventDays/7 || values[1] > maxEventDays || values[2] > maxEventDays*24 || values[3] > maxEventDays*24*60 || values[4] > maxEventDays*24*60*60 {
		return eventDuration{}, fmt.Errorf("duration exceeds %d days", maxEventDays)
	}
	duration := eventDuration{
		days:  values[0]*7 + values[1],
		clock: time.Duration(values[2])*time.Hour + time.Duration(values[3])*time.Minute + time.Duration(values[4])*time.Second,
	}
	if duration.days == 0 && duration.clock == 0 {
		return eventDuration{}, errors.New("duration must be positive")
	}
	if eventDurationLimit(duration) > maxEventDays*24*time.Hour {
		return eventDuration{}, fmt.Errorf("duration exceeds %d days", maxEventDays)
	}
	return duration, nil
}

func eventDurationLimit(value eventDuration) time.Duration {
	return time.Duration(value.days)*24*time.Hour + value.clock
}

func civilDayDifference(start, end time.Time) (int, error) {
	if end.Before(start) {
		return 0, errors.New("all-day DTEND precedes DTSTART")
	}
	cursor := start
	for days := 0; days <= maxEventDays; days++ {
		if cursor.Equal(end) {
			if days == 0 {
				return 0, errors.New("all-day event must span at least one day")
			}
			return days, nil
		}
		cursor = cursor.AddDate(0, 0, 1)
	}
	return 0, fmt.Errorf("all-day event exceeds %d days or changes time zone", maxEventDays)
}

func materializeEvents(parsed []parsedEvent, options ParseOptions) ([]Event, error) {
	masters := make(map[string]parsedEvent)
	exceptions := make(map[string]map[int64]parsedEvent)
	for _, event := range parsed {
		if event.recurrenceID == nil {
			if existing, found := masters[event.uid]; found {
				return nil, fmt.Errorf("VEVENT %d duplicates UID %q from VEVENT %d", event.componentNumber, event.uid, existing.componentNumber)
			}
			masters[event.uid] = event
			continue
		}
		key := event.recurrenceID.value.UTC().Unix()
		if exceptions[event.uid] == nil {
			exceptions[event.uid] = make(map[int64]parsedEvent)
		}
		if existing, found := exceptions[event.uid][key]; found {
			return nil, fmt.Errorf("VEVENT %d duplicates recurrence exception from VEVENT %d", event.componentNumber, existing.componentNumber)
		}
		exceptions[event.uid][key] = event
	}

	uids := make([]string, 0, len(masters))
	for uid := range masters {
		uids = append(uids, uid)
	}
	sort.Strings(uids)
	events := make([]Event, 0, len(parsed))
	for _, uid := range uids {
		master := masters[uid]
		if master.cancelled {
			continue
		}
		starts, err := recurrenceStarts(master, options.CoverageStart, options.CoverageEnd)
		if err != nil {
			return nil, fmt.Errorf("VEVENT %d recurrence: %w", master.componentNumber, err)
		}
		used := make(map[int64]bool)
		for _, start := range starts {
			selected := master
			if exception, found := exceptions[uid][start.UTC().Unix()]; found {
				if exception.recurrenceID.allDay != master.start.allDay || exception.start.allDay != master.start.allDay {
					return nil, fmt.Errorf("VEVENT %d recurrence exception changes DATE/DATE-TIME value type", exception.componentNumber)
				}
				used[start.UTC().Unix()] = true
				if exception.cancelled {
					continue
				}
				if !exception.hasDuration {
					exception.duration = master.duration
					exception.hasDuration = master.hasDuration
					exception.busy = exception.busy && eventDurationLimit(exception.duration) > 0
				}
				selected = exception
				start = exception.start.value
			}
			built, include, err := buildImportedEvent(selected, start, options)
			if err != nil {
				return nil, fmt.Errorf("VEVENT %d: %w", selected.componentNumber, err)
			}
			if include {
				events = append(events, built)
				if len(events) > MaxMaterializedEvents {
					return nil, fmt.Errorf("calendar materializes more than %d events", MaxMaterializedEvents)
				}
			}
		}
		for key, exception := range exceptions[uid] {
			if used[key] || exception.cancelled {
				continue
			}
			if exception.recurrenceID.allDay != master.start.allDay || exception.start.allDay != master.start.allDay {
				return nil, fmt.Errorf("VEVENT %d recurrence exception changes DATE/DATE-TIME value type", exception.componentNumber)
			}
			if !exception.hasDuration {
				exception.duration = master.duration
				exception.hasDuration = master.hasDuration
				exception.busy = exception.busy && eventDurationLimit(exception.duration) > 0
			}
			built, include, err := buildImportedEvent(exception, exception.start.value, options)
			if err != nil {
				return nil, fmt.Errorf("VEVENT %d: %w", exception.componentNumber, err)
			}
			if include {
				events = append(events, built)
				if len(events) > MaxMaterializedEvents {
					return nil, fmt.Errorf("calendar materializes more than %d events", MaxMaterializedEvents)
				}
			}
		}
	}

	for uid, detached := range exceptions {
		if _, found := masters[uid]; found {
			continue
		}
		for _, exception := range detached {
			if exception.cancelled {
				continue
			}
			built, include, err := buildImportedEvent(exception, exception.start.value, options)
			if err != nil {
				return nil, fmt.Errorf("VEVENT %d: %w", exception.componentNumber, err)
			}
			if include {
				events = append(events, built)
				if len(events) > MaxMaterializedEvents {
					return nil, fmt.Errorf("calendar materializes more than %d events", MaxMaterializedEvents)
				}
			}
		}
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].StartAt.Equal(events[j].StartAt) {
			return events[i].EventID < events[j].EventID
		}
		return events[i].StartAt.Before(events[j].StartAt)
	})
	return events, nil
}

func recurrenceStarts(event parsedEvent, coverageStart, coverageEnd time.Time) ([]time.Time, error) {
	if event.rrule == "" && len(event.rdates) == 0 {
		return []time.Time{event.start.value}, nil
	}
	set := &rrule.Set{}
	set.DTStart(event.start.value)
	if event.rrule == "" {
		set.RDate(event.start.value)
	} else {
		option, err := rrule.StrToROptionInLocation(event.rrule, event.start.value.Location())
		if err != nil {
			return nil, err
		}
		option.Dtstart = event.start.value
		if err := guardRecurrenceWork(*option, coverageEnd); err != nil {
			return nil, err
		}
		rule, err := rrule.NewRRule(*option)
		if err != nil {
			return nil, err
		}
		set.RRule(rule)
	}
	for _, value := range event.rdates {
		set.RDate(value)
	}
	for _, value := range event.exdates {
		set.ExDate(value)
	}
	after := coverageStart.Add(-eventDurationLimit(event.duration))
	starts := set.Between(after, coverageEnd, true)
	if len(starts) > MaxMaterializedEvents {
		return nil, fmt.Errorf("recurrence produces more than %d occurrences in coverage", MaxMaterializedEvents)
	}
	return starts, nil
}

func guardRecurrenceWork(option rrule.ROption, coverageEnd time.Time) error {
	if option.Freq == rrule.SECONDLY || option.Freq == rrule.MINUTELY {
		return errors.New("SECONDLY and MINUTELY recurrence rules are not supported")
	}
	if option.Count > maxRecurrenceWork {
		return fmt.Errorf("recurrence COUNT exceeds %d", maxRecurrenceWork)
	}
	end := coverageEnd
	if !option.Until.IsZero() && option.Until.Before(end) {
		end = option.Until
	}
	if !end.After(option.Dtstart) {
		return nil
	}
	interval := option.Interval
	if interval < 1 {
		interval = 1
	}
	var base float64
	switch option.Freq {
	case rrule.HOURLY:
		base = end.Sub(option.Dtstart).Hours() / float64(interval)
	case rrule.DAILY:
		base = end.Sub(option.Dtstart).Hours() / (24 * float64(interval))
	case rrule.WEEKLY:
		base = end.Sub(option.Dtstart).Hours() / (7 * 24 * float64(interval))
	case rrule.MONTHLY:
		base = end.Sub(option.Dtstart).Hours() / (28 * 24 * float64(interval))
	case rrule.YEARLY:
		base = end.Sub(option.Dtstart).Hours() / (365 * 24 * float64(interval))
	}
	expansion := max(1, len(option.Byhour)) * max(1, len(option.Byminute)) * max(1, len(option.Bysecond)) * max(1, len(option.Byweekday))
	if base*float64(expansion) > maxRecurrenceWork {
		return fmt.Errorf("recurrence requires more than %d bounded iterations", maxRecurrenceWork)
	}
	return nil
}

func buildImportedEvent(event parsedEvent, start time.Time, options ParseOptions) (Event, bool, error) {
	end := start.AddDate(0, 0, event.duration.days).Add(event.duration.clock)
	if !intervalIntersects(start, end, options.CoverageStart, options.CoverageEnd) {
		return Event{}, false, nil
	}
	recordID := event.uid + "/" + start.UTC().Format("20060102T150405Z")
	built := Event{
		EventID:        deterministicEventID(options.SourceID, event.uid, start),
		SourceID:       options.SourceID,
		SourceRecordID: recordID,
		Title:          event.title,
		StartAt:        start.UTC(),
		EndAt:          end.UTC(),
		ZoneID:         event.start.zoneID,
		AllDay:         event.start.allDay,
		Busy:           event.busy,
		Ownership:      OwnershipImported,
		CreatedAt:      options.ImportedAt.UTC(),
		Location:       event.location,
		Notes:          event.notes,
	}
	if err := built.Validate(); err != nil {
		return Event{}, false, err
	}
	return built, true, nil
}

func intervalIntersects(start, end, coverageStart, coverageEnd time.Time) bool {
	if start.Equal(end) {
		return !start.Before(coverageStart) && start.Before(coverageEnd)
	}
	return end.After(coverageStart) && start.Before(coverageEnd)
}

func deterministicEventID(sourceID, uid string, start time.Time) string {
	sum := sha256.Sum256([]byte(sourceID + "\x00" + uid + "\x00" + start.UTC().Format(time.RFC3339Nano)))
	return "calendar_event_" + hex.EncodeToString(sum[:16])
}
