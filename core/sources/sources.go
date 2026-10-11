// Package sources defines the source report of
// contracts/v1/source-report-set: what a device found when it checked a
// source of sleep records (ADR-0054). A report is defined and validated once,
// here, for the server's sync validation and the desktop's store, and this
// package also reads the reports into each source's status. Reports carry no
// sleep and are not estimator input.
package sources

import (
	"errors"
	"regexp"
	"sort"
	"time"
)

// Sources that report.
const HealthConnect = "health_connect"

// What a device found.
const (
	// Read: the source was read in full over the covered window.
	Read = "read"
	// PermissionMissing: the device may not read the source.
	PermissionMissing = "permission_missing"
	// Unavailable: the source cannot be used on the device.
	Unavailable = "unavailable"
	// ReadFailed: a read was attempted and failed.
	ReadFailed = "read_failed"
)

const maxHeld = 10000

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)

type Report struct {
	ReportID        string    `json:"report_id"`
	Source          string    `json:"source"`
	Outcome         string    `json:"outcome"`
	ReportedAt      time.Time `json:"reported_at"`
	BackgroundReads bool      `json:"background_reads"`
	// Covered and Held are given with a read, and only then.
	Covered *Window `json:"covered,omitempty"`
	Held    *int    `json:"held,omitempty"`
}

// Window is a half-open interval of instants.
type Window struct {
	StartAt time.Time `json:"start_at"`
	EndAt   time.Time `json:"end_at"`
}

func ValidOutcome(value string) bool {
	switch value {
	case Read, PermissionMissing, Unavailable, ReadFailed:
		return true
	default:
		return false
	}
}

func (report Report) Validate() error {
	if !identifier.MatchString(report.ReportID) {
		return errors.New("report_id must match the v1 identifier format")
	}
	if report.Source != HealthConnect {
		return errors.New("source must be health_connect")
	}
	if !ValidOutcome(report.Outcome) {
		return errors.New("outcome must be read, permission_missing, unavailable, or read_failed")
	}
	if report.ReportedAt.IsZero() {
		return errors.New("reported_at is required")
	}
	if report.Outcome != Read {
		if report.Covered != nil || report.Held != nil {
			return errors.New("only a read states a covered window and a held count")
		}
		return nil
	}
	if report.Covered == nil || report.Held == nil {
		return errors.New("a read states its covered window and its held count")
	}
	if !report.Covered.EndAt.After(report.Covered.StartAt) {
		return errors.New("covered.end_at must be after covered.start_at")
	}
	if report.Covered.EndAt.After(report.ReportedAt) {
		return errors.New("covered.end_at cannot be after reported_at")
	}
	if *report.Held < 0 || *report.Held > maxHeld {
		return errors.New("held must be between 0 and 10000")
	}
	return nil
}

// Received is a report as it arrived: which device sent it.
type Received struct {
	DeviceID string
	Report   Report
}

// Status is what a source's reports say about it now, from the device that
// reported last.
type Status struct {
	Source   string
	DeviceID string
	// Outcome, ReportedAt and BackgroundReads are those of the newest report.
	Outcome         string
	ReportedAt      time.Time
	BackgroundReads bool
	// Since is when the newest outcome began: the first report of the
	// unbroken run of reports with that outcome.
	Since time.Time
	// ReadThrough is the end of the newest read window; zero if never read.
	ReadThrough time.Time
	// Held is the newest read's count of records held back.
	Held int
}

// Statuses reads each source's status from its reports, one per source, in
// source order. Where several devices report a source, the one that reported
// last speaks for it.
func Statuses(received []Received) []Status {
	type key struct{ source, device string }
	byDevice := map[key][]Report{}
	for _, item := range received {
		k := key{item.Report.Source, item.DeviceID}
		byDevice[k] = append(byDevice[k], item.Report)
	}
	bySource := map[string]Status{}
	for k, reports := range byDevice {
		status := statusOf(reports)
		status.Source, status.DeviceID = k.source, k.device
		current, ok := bySource[k.source]
		if !ok || status.ReportedAt.After(current.ReportedAt) ||
			(status.ReportedAt.Equal(current.ReportedAt) && status.DeviceID > current.DeviceID) {
			bySource[k.source] = status
		}
	}
	statuses := make([]Status, 0, len(bySource))
	for _, status := range bySource {
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Source < statuses[j].Source })
	return statuses
}

// statusOf reads one device's reports about one source.
func statusOf(reports []Report) Status {
	sort.Slice(reports, func(i, j int) bool {
		if !reports[i].ReportedAt.Equal(reports[j].ReportedAt) {
			return reports[i].ReportedAt.Before(reports[j].ReportedAt)
		}
		return reports[i].ReportID < reports[j].ReportID
	})
	newest := reports[len(reports)-1]
	status := Status{
		Outcome:         newest.Outcome,
		ReportedAt:      newest.ReportedAt.UTC(),
		BackgroundReads: newest.BackgroundReads,
		Since:           newest.ReportedAt.UTC(),
	}
	for index := len(reports) - 1; index >= 0 && reports[index].Outcome == newest.Outcome; index-- {
		status.Since = reports[index].ReportedAt.UTC()
	}
	for index := len(reports) - 1; index >= 0; index-- {
		if read := reports[index]; read.Outcome == Read && read.Covered != nil {
			status.ReadThrough = read.Covered.EndAt.UTC()
			if read.Held != nil {
				status.Held = *read.Held
			}
			break
		}
	}
	return status
}
