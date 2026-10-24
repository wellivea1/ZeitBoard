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

// calendarWalk follows a calendar's lines: the components open, the event
// being collected, and what the root has shown so far.
type calendarWalk struct {
	stack          []string
	components     []component
	current        *component
	seenCalendar   bool
	closedCalendar bool
	versionSeen    bool
}

func collectEventComponents(lines []contentLine) ([]component, error) {
	walk := calendarWalk{stack: make([]string, 0, 4), components: make([]component, 0)}
	for _, line := range lines {
		var err error
		switch line.name {
		case "BEGIN":
			err = walk.begin(line)
		case "END":
			err = walk.end(line)
		default:
			err = walk.property(line)
		}
		if err != nil {
			return nil, err
		}
	}
	return walk.finish()
}

// begin opens a component: the one VCALENDAR root, or an event directly in it.
func (w *calendarWalk) begin(line contentLine) error {
	name := strings.ToUpper(strings.TrimSpace(line.value))
	if name == "" {
		return fmt.Errorf("line %d: BEGIN component is empty", line.line)
	}
	if len(w.stack) == 0 {
		if name != "VCALENDAR" || w.seenCalendar || w.closedCalendar {
			return fmt.Errorf("line %d: expected one VCALENDAR root", line.line)
		}
		w.seenCalendar = true
	}
	if name == "VEVENT" {
		if len(w.stack) == 0 || w.stack[len(w.stack)-1] != "VCALENDAR" || w.current != nil {
			return fmt.Errorf("line %d: VEVENT must be a direct VCALENDAR child", line.line)
		}
		w.current = &component{index: len(w.components) + 1}
	}
	w.stack = append(w.stack, name)
	return nil
}

// end closes the component open last, keeping an event it closes.
func (w *calendarWalk) end(line contentLine) error {
	name := strings.ToUpper(strings.TrimSpace(line.value))
	if len(w.stack) == 0 || w.stack[len(w.stack)-1] != name {
		return fmt.Errorf("line %d: END:%s does not match the open component", line.line, name)
	}
	if name == "VEVENT" {
		w.components = append(w.components, *w.current)
		w.current = nil
	}
	w.stack = w.stack[:len(w.stack)-1]
	if name == "VCALENDAR" {
		w.closedCalendar = true
	}
	return nil
}

// property reads a line inside a component: the root's one VERSION, or a
// line of the event being collected.
func (w *calendarWalk) property(line contentLine) error {
	if len(w.stack) == 0 {
		return fmt.Errorf("line %d: property appears outside VCALENDAR", line.line)
	}
	if len(w.stack) == 1 && w.stack[0] == "VCALENDAR" && line.name == "VERSION" {
		if w.versionSeen {
			return fmt.Errorf("line %d: VERSION appears more than once", line.line)
		}
		if strings.TrimSpace(line.value) != "2.0" {
			return fmt.Errorf("line %d: unsupported iCalendar VERSION %q", line.line, line.value)
		}
		w.versionSeen = true
	}
	if w.current != nil && w.stack[len(w.stack)-1] == "VEVENT" {
		w.current.lines = append(w.current.lines, line)
	}
	return nil
}

// finish gives the events once the calendar is whole: its root closed, and
// its version stated.
func (w *calendarWalk) finish() ([]component, error) {
	if len(w.stack) != 0 {
		return nil, fmt.Errorf("calendar ended with unclosed %s component", w.stack[len(w.stack)-1])
	}
	if !w.seenCalendar || !w.closedCalendar {
		return nil, errors.New("calendar is missing a complete VCALENDAR root")
	}
	if !w.versionSeen {
		return nil, errors.New("calendar is missing VERSION:2.0")
	}
	return w.components, nil
}

func parseEventComponent(item component, defaultZoneID string) (parsedEvent, error) {
	uid, err := eventUID(item.lines)
	if err != nil {
		return parsedEvent{}, err
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
	// In this order, so a calendar with several faults is refused for the
	// same one every time.
	for _, read := range []func(*parsedEvent, []contentLine, string) error{
		readEventText, readEventStatus, readEventDuration, readEventRecurrence,
	} {
		if err := read(&event, item.lines, defaultZoneID); err != nil {
			return parsedEvent{}, err
		}
	}
	return event, nil
}

// eventUID reads the one UID an event must have.
func eventUID(lines []contentLine) (string, error) {
	line, err := exactlyOne(lines, "UID", true)
	if err != nil {
		return "", err
	}
	uid := strings.TrimSpace(line.value)
	if err := boundedText("UID", uid, 1, 400); err != nil {
		return "", err
	}
	if strings.ContainsAny(uid, "\r\n\x00") {
		return "", errors.New("UID contains a control character")
	}
	return uid, nil
}

// readEventText reads the title, location and notes, each at most once. An
// event without a title is "Busy".
func readEventText(event *parsedEvent, lines []contentLine, _ string) error {
	title, err := optionalText(lines, "SUMMARY")
	if err != nil {
		return err
	}
	if title != "" {
		event.title = title
	}
	if err := boundedText("event title", event.title, 1, maxTitleRunes); err != nil {
		return err
	}
	if event.location, err = optionalText(lines, "LOCATION"); err != nil {
		return err
	}
	if event.location != "" {
		if err := boundedText("event location", event.location, 1, maxLocationRunes); err != nil {
			return err
		}
	}
	if event.notes, err = optionalText(lines, "DESCRIPTION"); err != nil {
		return err
	}
	if event.notes != "" {
		if err := boundedText("event notes", event.notes, 1, maxNotesRunes); err != nil {
			return err
		}
	}
	return nil
}

// optionalText is a text property's decoded and trimmed value, or "" when
// the event has none. It may appear at most once.
func optionalText(lines []contentLine, name string) (string, error) {
	line, err := exactlyOne(lines, name, false)
	if err != nil || line.name == "" {
		return "", err
	}
	text, err := decodeText(line.value)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return strings.TrimSpace(text), nil
}

// readEventStatus reads STATUS and TRANSP. A cancelled event, or a
// transparent one, is not busy.
func readEventStatus(event *parsedEvent, lines []contentLine, _ string) error {
	if line, err := exactlyOne(lines, "STATUS", false); err != nil {
		return err
	} else if line.name != "" {
		switch strings.ToUpper(strings.TrimSpace(line.value)) {
		case "CANCELLED":
			event.cancelled = true
			event.busy = false
		case "CONFIRMED", "TENTATIVE":
		default:
			return fmt.Errorf("unsupported STATUS %q", line.value)
		}
	}
	if line, err := exactlyOne(lines, "TRANSP", false); err != nil {
		return err
	} else if line.name != "" {
		switch strings.ToUpper(strings.TrimSpace(line.value)) {
		case "TRANSPARENT":
			event.busy = false
		case "OPAQUE":
		default:
			return fmt.Errorf("unsupported TRANSP %q", line.value)
		}
	}
	return nil
}

// readEventDuration reads how long an event lasts, from DTEND or DURATION
// but never both. An all-day event with neither lasts a day. A timed one with
// neither, or one that lasts no time, is not busy.
func readEventDuration(event *parsedEvent, lines []contentLine, defaultZoneID string) error {
	endLine, err := exactlyOne(lines, "DTEND", false)
	if err != nil {
		return err
	}
	durationLine, err := exactlyOne(lines, "DURATION", false)
	if err != nil {
		return err
	}
	if endLine.name != "" && durationLine.name != "" {
		return errors.New("DTEND and DURATION are mutually exclusive")
	}
	switch {
	case endLine.name != "":
		if err := readEventEnd(event, endLine, defaultZoneID); err != nil {
			return err
		}
	case durationLine.name != "":
		event.duration, err = parseICalDuration(durationLine.value)
		if err != nil {
			return fmt.Errorf("DURATION: %w", err)
		}
		event.hasDuration = true
	case event.start.allDay:
		event.duration.days = 1
		event.hasDuration = true
	default:
		event.busy = false
	}
	if eventDurationLimit(event.duration) > maxEventDays*24*time.Hour {
		return fmt.Errorf("event duration exceeds %d days", maxEventDays)
	}
	if event.busy && event.duration.days == 0 && event.duration.clock == 0 {
		event.busy = false
	}
	return nil
}

// readEventEnd reads DTEND, which must be of DTSTART's kind and not before
// it.
func readEventEnd(event *parsedEvent, line contentLine, defaultZoneID string) error {
	start := event.start
	end, err := parseTemporal(line, defaultZoneID)
	if err != nil {
		return fmt.Errorf("DTEND: %w", err)
	}
	if start.allDay != end.allDay {
		return errors.New("DTSTART and DTEND must use the same value type")
	}
	if end.value.Before(start.value) {
		return errors.New("DTEND precedes DTSTART")
	}
	if start.allDay {
		days, err := civilDayDifference(start.value, end.value)
		if err != nil {
			return err
		}
		event.duration.days = days
	} else {
		event.duration.clock = end.value.Sub(start.value)
	}
	event.hasDuration = true
	return nil
}

// readEventRecurrence reads what makes an event recur, or an exception to
// one. An exception names the occurrence it replaces, and recurs no further.
func readEventRecurrence(event *parsedEvent, lines []contentLine, defaultZoneID string) error {
	if line, err := exactlyOne(lines, "RECURRENCE-ID", false); err != nil {
		return err
	} else if line.name != "" {
		if values := line.params["RANGE"]; len(values) != 0 {
			return errors.New("RECURRENCE-ID RANGE is not supported")
		}
		value, err := parseTemporal(line, defaultZoneID)
		if err != nil {
			return fmt.Errorf("RECURRENCE-ID: %w", err)
		}
		event.recurrenceID = &value
	}
	if line, err := exactlyOne(lines, "RRULE", false); err != nil {
		return err
	} else if line.name != "" {
		event.rrule = strings.TrimSpace(line.value)
	}
	if event.recurrenceID != nil && event.rrule != "" {
		return errors.New("a recurrence exception cannot define RRULE")
	}

	for _, line := range matching(lines, "RDATE") {
		values, err := parseTemporalList(line, defaultZoneID, event.start.allDay)
		if err != nil {
			return fmt.Errorf("RDATE: %w", err)
		}
		event.rdates = append(event.rdates, values...)
	}
	for _, line := range matching(lines, "EXDATE") {
		values, err := parseTemporalList(line, defaultZoneID, event.start.allDay)
		if err != nil {
			return fmt.Errorf("EXDATE: %w", err)
		}
		event.exdates = append(event.exdates, values...)
	}
	if event.recurrenceID != nil && (len(event.rdates) != 0 || len(event.exdates) != 0) {
		return errors.New("a recurrence exception cannot define RDATE or EXDATE")
	}
	return nil
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
	masters, exceptions, err := groupEvents(parsed)
	if err != nil {
		return nil, err
	}
	uids := make([]string, 0, len(masters))
	for uid := range masters {
		uids = append(uids, uid)
	}
	sort.Strings(uids)
	m := materializer{options: options, events: make([]Event, 0, len(parsed))}
	for _, uid := range uids {
		if err := m.series(masters[uid], exceptions[uid]); err != nil {
			return nil, err
		}
	}
	// An exception whose series is not in the calendar stands on its own.
	for uid, detached := range exceptions {
		if _, found := masters[uid]; found {
			continue
		}
		for _, exception := range detached {
			if exception.cancelled {
				continue
			}
			if err := m.add(exception, exception.start.value); err != nil {
				return nil, err
			}
		}
	}

	events := m.events
	sort.Slice(events, func(i, j int) bool {
		if events[i].StartAt.Equal(events[j].StartAt) {
			return events[i].EventID < events[j].EventID
		}
		return events[i].StartAt.Before(events[j].StartAt)
	})
	return events, nil
}

// groupEvents sorts a calendar's events into series by UID, with each
// series' exceptions by the start they replace. A UID, or an exception,
// given twice is refused.
func groupEvents(parsed []parsedEvent) (map[string]parsedEvent, map[string]map[int64]parsedEvent, error) {
	masters := make(map[string]parsedEvent)
	exceptions := make(map[string]map[int64]parsedEvent)
	for _, event := range parsed {
		if event.recurrenceID == nil {
			if existing, found := masters[event.uid]; found {
				return nil, nil, fmt.Errorf("VEVENT %d duplicates UID %q from VEVENT %d", event.componentNumber, event.uid, existing.componentNumber)
			}
			masters[event.uid] = event
			continue
		}
		key := event.recurrenceID.value.UTC().Unix()
		if exceptions[event.uid] == nil {
			exceptions[event.uid] = make(map[int64]parsedEvent)
		}
		if existing, found := exceptions[event.uid][key]; found {
			return nil, nil, fmt.Errorf("VEVENT %d duplicates recurrence exception from VEVENT %d", event.componentNumber, existing.componentNumber)
		}
		exceptions[event.uid][key] = event
	}
	return masters, exceptions, nil
}

// materializer gathers the events a calendar gives, up to the cap.
type materializer struct {
	options ParseOptions
	events  []Event
}

// add builds the event that starts at start, if the coverage holds it.
func (m *materializer) add(event parsedEvent, start time.Time) error {
	built, include, err := buildImportedEvent(event, start, m.options)
	if err != nil {
		return fmt.Errorf("VEVENT %d: %w", event.componentNumber, err)
	}
	if !include {
		return nil
	}
	m.events = append(m.events, built)
	if len(m.events) > MaxMaterializedEvents {
		return fmt.Errorf("calendar materializes more than %d events", MaxMaterializedEvents)
	}
	return nil
}

// series adds a series' occurrences, each replaced by its exception when it
// has one, and then the exceptions that replace no occurrence the series
// gave.
func (m *materializer) series(master parsedEvent, exceptions map[int64]parsedEvent) error {
	if master.cancelled {
		return nil
	}
	starts, err := recurrenceStarts(master, m.options.CoverageStart, m.options.CoverageEnd)
	if err != nil {
		return fmt.Errorf("VEVENT %d recurrence: %w", master.componentNumber, err)
	}
	used := make(map[int64]bool)
	for _, start := range starts {
		selected := master
		if exception, found := exceptions[start.UTC().Unix()]; found {
			fitted, err := fitException(master, exception)
			if err != nil {
				return err
			}
			used[start.UTC().Unix()] = true
			if exception.cancelled {
				continue
			}
			selected, start = fitted, fitted.start.value
		}
		if err := m.add(selected, start); err != nil {
			return err
		}
	}
	for key, exception := range exceptions {
		if used[key] || exception.cancelled {
			continue
		}
		fitted, err := fitException(master, exception)
		if err != nil {
			return err
		}
		if err := m.add(fitted, fitted.start.value); err != nil {
			return err
		}
	}
	return nil
}

// fitException checks an exception against its series, whose kind of start
// it must keep, and gives it the series' length when it states none.
func fitException(master, exception parsedEvent) (parsedEvent, error) {
	if exception.recurrenceID.allDay != master.start.allDay || exception.start.allDay != master.start.allDay {
		return parsedEvent{}, fmt.Errorf("VEVENT %d recurrence exception changes DATE/DATE-TIME value type", exception.componentNumber)
	}
	if !exception.hasDuration {
		exception.duration = master.duration
		exception.hasDuration = master.hasDuration
		exception.busy = exception.busy && eventDurationLimit(exception.duration) > 0
	}
	return exception, nil
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
