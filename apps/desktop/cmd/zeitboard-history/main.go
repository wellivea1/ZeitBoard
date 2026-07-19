// Command zeitboard-history converts owner-controlled source files into the
// v1 sleep observation format, previews/imports them locally, and runs the
// estimator validation workflow without placing raw health data in the repo.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	storage "non24.app/core/storage/sqlite"
	"non24.app/desktop/internal/history"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "template":
		err = runTemplate(os.Args[2:])
	case "fitbit":
		err = runFitbit(os.Args[2:])
	case "transcription":
		err = runTranscription(os.Args[2:])
	case "import":
		err = runImport(os.Args[2:])
	case "backtest":
		err = runBacktest(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runTemplate(args []string) error {
	flags := flag.NewFlagSet("template", flag.ContinueOnError)
	output := flags.String("out", "", "output CSV path")
	from := flags.String("from", "", "optional first chart date, YYYY-MM-DD")
	through := flags.String("through", "", "optional last chart date, YYYY-MM-DD")
	zoneID := flags.String("zone", "America/New_York", "IANA time zone for generated review rows")
	sourcePrefix := flags.String("source-prefix", "chart", "stable source_record_id prefix for generated review rows")
	force := flags.Bool("force", false, "replace an existing output file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*output) == "" {
		return errors.New("template requires --out")
	}
	encoded, rows, err := history.EncodeTranscriptionTemplate(history.TranscriptionTemplateOptions{
		FromDate:     *from,
		ThroughDate:  *through,
		ZoneID:       *zoneID,
		SourcePrefix: *sourcePrefix,
	})
	if err != nil {
		return err
	}
	if err := writePrivateFile(*output, encoded, *force); err != nil {
		return err
	}
	if rows == 0 {
		fmt.Printf("Wrote header-only owner transcription template to %s\n", *output)
	} else {
		fmt.Printf("Wrote %d needs_review chart rows to %s\n", rows, *output)
	}
	return nil
}

func runFitbit(args []string) error {
	flags := flag.NewFlagSet("fitbit", flag.ContinueOnError)
	input := flags.String("in", "", "directory containing Fitbit Sleep Data CSV exports")
	output := flags.String("out", "", "output v1 observation-set path")
	format := flags.String("format", "json", "output format: json or csv")
	zoneID := flags.String("zone", "America/New_York", "IANA time zone for Fitbit civil timestamps")
	from := flags.String("from", "2021-01-01", "first local start date, YYYY-MM-DD")
	through := flags.String("through", "2023-12-31", "last local start date, YYYY-MM-DD")
	includeSuperseded := flags.Bool("include-superseded", false, "include files under Old, Incomplete, and weekly directories")
	force := flags.Bool("force", false, "replace an existing output file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*input) == "" || strings.TrimSpace(*output) == "" {
		return errors.New("fitbit requires --in and --out")
	}
	observations, report, err := history.ConvertFitbitDirectory(history.FitbitOptions{
		Directory:         *input,
		ZoneID:            *zoneID,
		FromDate:          *from,
		ThroughDate:       *through,
		IncludeSuperseded: *includeSuperseded,
	})
	if err != nil {
		return err
	}
	encoded, err := history.EncodeObservationSet(*format, time.Now().UTC(), observations)
	if err != nil {
		return err
	}
	if err := writePrivateFile(*output, encoded, *force); err != nil {
		return err
	}
	napSummary := "rows under 3h classified as naps"
	if report.NapRows == 1 {
		napSummary = "row under 3h classified as a nap"
	}
	fmt.Printf("Read %d rows from %d finalized Fitbit files: %d observations, %d exact duplicates, %d outside range, %d %s.\n",
		report.RowsRead, report.FilesRead, report.Observations, report.DuplicateRows, report.RowsOutsideRange, report.NapRows, napSummary)
	if report.FilesIgnored > 0 {
		fmt.Printf("Excluded %d matching files under Old, Incomplete, or weekly directories; use --include-superseded for comparison.\n", report.FilesIgnored)
	}
	if !report.EarliestStart.IsZero() {
		fmt.Printf("Included local start dates %s through %s.\n", report.EarliestStart.Format(time.DateOnly), report.LatestStart.Format(time.DateOnly))
	}
	fmt.Printf("Wrote private v1 import file to %s\n", *output)
	return nil
}

func runTranscription(args []string) error {
	flags := flag.NewFlagSet("transcription", flag.ContinueOnError)
	input := flags.String("in", "", "completed owner transcription template")
	output := flags.String("out", "", "output v1 observation-set path")
	format := flags.String("format", "json", "output format: json or csv")
	force := flags.Bool("force", false, "replace an existing output file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*input) == "" || strings.TrimSpace(*output) == "" {
		return errors.New("transcription requires --in and --out")
	}
	observations, report, err := history.ConvertTranscriptionFile(*input)
	if err != nil {
		fmt.Printf("Review rows: total=%d confirmed_sleep=%d confirmed_no_observation=%d pending=%d\n",
			report.Rows, report.Observations, report.NoObservationRows, report.PendingRows)
		return err
	}
	encoded, err := history.EncodeObservationSet(*format, time.Now().UTC(), observations)
	if err != nil {
		return err
	}
	if err := writePrivateFile(*output, encoded, *force); err != nil {
		return err
	}
	fmt.Printf("Converted %d owner-reviewed rows into %d observations; %d rows explicitly confirmed no observation.\n",
		report.Rows, report.Observations, report.NoObservationRows)
	fmt.Printf("Wrote private v1 import file to %s\n", *output)
	return nil
}

func runImport(args []string) error {
	flags := flag.NewFlagSet("import", flag.ContinueOnError)
	input := flags.String("in", "", "v1 JSON or canonical CSV import file")
	database := flags.String("database", "", "local SQLite database path")
	commit := flags.Bool("commit", false, "append ready rows after a successful preview")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*input) == "" || strings.TrimSpace(*database) == "" {
		return errors.New("import requires --in and --database")
	}
	info, err := os.Stat(*input)
	if err != nil {
		return err
	}
	if info.Size() > storage.MaxSleepImportBytes {
		return fmt.Errorf("import file exceeds the %d MiB limit", storage.MaxSleepImportBytes/(1024*1024))
	}
	contents, err := os.ReadFile(*input)
	if err != nil {
		return err
	}
	store, err := storage.Open(*database)
	if err != nil {
		return err
	}
	defer store.Close()
	importInput := storage.SleepImportInput{FileName: filepath.Base(*input), Contents: string(contents)}
	var report storage.SleepImportReport
	if *commit {
		report, err = store.ImportSleepObservations(context.Background(), importInput)
	} else {
		report, err = store.PreviewSleepImport(context.Background(), importInput)
	}
	if err != nil {
		return err
	}
	printImportReport(report)
	if report.InvalidRows > 0 || len(report.Errors) > 0 {
		return errors.New("import validation failed; no observations were written")
	}
	return nil
}

func runBacktest(args []string) error {
	flags := flag.NewFlagSet("backtest", flag.ContinueOnError)
	database := flags.String("database", "", "local SQLite database path")
	output := flags.String("out", "", "optional aggregate Markdown output path")
	force := flags.Bool("force", false, "replace an existing output file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*database) == "" {
		return errors.New("backtest requires --database")
	}
	store, err := storage.Open(*database)
	if err != nil {
		return err
	}
	defer store.Close()
	sessions, err := store.EffectiveSleepSessions(context.Background())
	if err != nil {
		return err
	}
	summaries, err := history.RunBacktestMatrix(context.Background(), sessions)
	if err != nil {
		return err
	}
	markdown := history.FormatBacktestMarkdown(summaries)
	fmt.Print(markdown)
	if strings.TrimSpace(*output) != "" {
		if err := writePrivateFile(*output, []byte(markdown), *force); err != nil {
			return err
		}
		fmt.Printf("Wrote aggregate backtest report to %s\n", *output)
	}
	return nil
}

func printImportReport(report storage.SleepImportReport) {
	action := "Preview"
	if !report.DryRun {
		action = "Import"
	}
	fmt.Printf("%s: %s\n", action, report.Message)
	fmt.Printf("Rows: total=%d ready=%d duplicates=%d invalid=%d imported=%d\n",
		report.TotalRows, report.ReadyRows, report.DuplicateRows, report.InvalidRows, report.ImportedRows)
	for _, problem := range report.Errors {
		fmt.Printf("File error: %s\n", problem)
	}
	for _, row := range report.Rows {
		if len(row.Errors) == 0 {
			continue
		}
		fmt.Printf("Row %d: %s\n", row.RowNumber, strings.Join(row.Errors, "; "))
	}
}

func writePrivateFile(path string, data []byte, force bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE
	if force {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: zeitboard-history <command> [options]

Commands:
  template       Write the header-only owner transcription CSV template
  fitbit         Convert overlapping Fitbit Sleep Data CSV exports to v1
  transcription Convert an owner-reviewed transcription template to v1
  import         Dry-run (default) or commit a v1 JSON/CSV file to SQLite
  backtest       Compare baseline and tightened windows on imported history`)
}
