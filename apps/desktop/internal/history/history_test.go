package history

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"non24.app/core/domain"
	storage "non24.app/core/storage/sqlite"
)

func TestFitbitConverterAccountsForOverlappingFilesAndProducesImportableV1(t *testing.T) {
	directory := t.TempDir()
	header := "Sleep\nStart Time,End Time,Minutes Asleep,Minutes Awake,Number of Awakenings,Time in Bed,Minutes REM Sleep,Minutes Light Sleep,Minutes Deep Sleep\n"
	principal := `"2023-03-01 11:00PM","2023-03-02 7:00AM","420","60","20","480","90","250","80"` + "\n"
	nap := `"2023-03-02 1:00PM","2023-03-02 2:30PM","80","10","2","90","N/A","N/A","N/A"` + "\n"
	writeHistoryTestFile(t, filepath.Join(directory, "Fitbit Sleep Data A.csv"), header+principal+nap)
	writeHistoryTestFile(t, filepath.Join(directory, "Fitbit Sleep Data B.csv"), header+principal)
	if err := os.Mkdir(filepath.Join(directory, "Old"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeHistoryTestFile(t, filepath.Join(directory, "Old", "Fitbit Sleep Data stale.csv"), header+principal)

	observations, report, err := ConvertFitbitDirectory(FitbitOptions{
		Directory: directory, ZoneID: "America/New_York", FromDate: "2023-01-01", ThroughDate: "2023-12-31",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.FilesRead != 2 || report.FilesIgnored != 1 || report.RowsRead != 3 || report.DuplicateRows != 1 || report.Observations != 2 || report.NapRows != 1 {
		t.Fatalf("unexpected Fitbit accounting: %#v", report)
	}
	encoded, err := EncodeObservationSet("json", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), observations)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(t.TempDir(), "fitbit-import.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	preview, err := store.PreviewSleepImport(context.Background(), storage.SleepImportInput{FileName: "fitbit.json", Contents: string(encoded)})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.CanImport || preview.ReadyRows != 2 || preview.InvalidRows != 0 {
		t.Fatalf("converted Fitbit output is not importable: %#v", preview)
	}
}

func TestFitbitConverterRejectsAmbiguousCivilTimes(t *testing.T) {
	directory := t.TempDir()
	header := "Sleep\nStart Time,End Time,Minutes Asleep,Minutes Awake,Number of Awakenings,Time in Bed,Minutes REM Sleep,Minutes Light Sleep,Minutes Deep Sleep\n"
	ambiguous := `"2023-11-05 1:30AM","2023-11-05 9:30AM","420","60","20","480","90","250","80"` + "\n"
	writeHistoryTestFile(t, filepath.Join(directory, "Fitbit Sleep Data DST.csv"), header+ambiguous)

	_, _, err := ConvertFitbitDirectory(FitbitOptions{
		Directory: directory, ZoneID: "America/New_York", FromDate: "2023-01-01", ThroughDate: "2023-12-31",
	})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous Fitbit civil time was not rejected explicitly: %v", err)
	}
}

func TestTranscriptionConverterRejectsAmbiguousTimesAndRepeatedSourceIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcription.csv")
	writeHistoryTestFile(t, path, TranscriptionTemplate+
		"chart-2021-11-07,2021-11-07 01:30,2021-11-07 09:30,America/New_York,principal\n"+
		"chart-2021-11-07,2021-11-08 01:30,2021-11-08 09:30,America/New_York,principal\n")
	_, report, err := ConvertTranscriptionFile(path)
	if err == nil {
		t.Fatal("ambiguous and repeated transcription rows should fail")
	}
	if report.Rows != 2 || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("transcription errors were not explicit: report=%#v err=%v", report, err)
	}
}

func TestTranscriptionConverterRejectsAnExplicitOffsetOutsideTheNamedZone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcription.csv")
	writeHistoryTestFile(t, path, TranscriptionTemplate+
		"chart-2021-offset,2021-03-11T21:00:00+03:00,2021-03-12T07:00:00+03:00,America/New_York,principal\n")
	_, _, err := ConvertTranscriptionFile(path)
	if err == nil || !strings.Contains(err.Error(), "does not match zone_id") {
		t.Fatalf("mismatched explicit offset was not rejected: %v", err)
	}
}

func TestTranscriptionConverterProducesCanonicalCSV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcription.csv")
	writeHistoryTestFile(t, path, TranscriptionTemplate+
		"chart-2021-03-11,2021-03-11 21:00,2021-03-12 07:00,America/New_York,principal\n")
	observations, report, err := ConvertTranscriptionFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Rows != 1 || report.Observations != 1 || observations[0].Provenance.EvidenceStatus != storage.ProvenanceEvidenceUserReported {
		t.Fatalf("unexpected transcription conversion: report=%#v observations=%#v", report, observations)
	}
	encoded, err := EncodeObservationSet("csv", time.Time{}, observations)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(t.TempDir(), "transcription-import.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	preview, err := store.PreviewSleepImport(context.Background(), storage.SleepImportInput{FileName: "transcription-v1.csv", Contents: string(encoded)})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.CanImport || preview.ReadyRows != 1 {
		t.Fatalf("canonical transcription CSV is not importable: %#v", preview)
	}
}

func TestBacktestMatrixMeasuresWindowTighteningOnTheSamePoints(t *testing.T) {
	sessions := make([]domain.SleepSession, 0, 30)
	start := time.Date(2023, 1, 1, 5, 0, 0, 0, time.UTC)
	for index := 0; index < 30; index++ {
		onset := start.Add(time.Duration(index) * (25*time.Hour + 5*time.Minute))
		instant := domain.MustZonedInstant(onset, "UTC")
		wake := domain.MustZonedInstant(onset.Add(8*time.Hour), "UTC")
		evidence := domain.Evidence{Acquisition: domain.AcquisitionImported, Status: domain.StatusObserved}
		sessions = append(sessions, domain.SleepSession{
			ID: domain.SleepSessionID(fmt.Sprintf("owner-%02d", index)),
			Intervals: []domain.SleepInterval{{
				Interval:      domain.TimeRange{Start: instant, End: wake},
				StartEvidence: evidence,
				EndEvidence:   evidence,
			}},
		})
	}
	summaries, err := RunBacktestMatrix(context.Background(), sessions)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 3 || summaries[0].Evaluations != summaries[2].Evaluations {
		t.Fatalf("candidates did not evaluate identical points: %#v", summaries)
	}
	if summaries[2].MeanWindowWidthHours >= summaries[0].MeanWindowWidthHours {
		t.Fatalf("tightening candidate did not reduce mean window width: %#v", summaries)
	}
	markdown := FormatBacktestMarkdown(summaries)
	if !strings.Contains(markdown, "| baseline |") {
		t.Fatal("markdown report omitted baseline")
	}
	if !strings.Contains(markdown, "| baseline | none | 0 |") {
		t.Fatal("markdown report omitted refusal accounting")
	}
}

func writeHistoryTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
