import type {
  MedicationClinicalReport,
  MedicationClinicalReportExport,
  MedicationClinicalReportInput,
} from "../data/medicationReport";

function civilDate(index: number): string {
  const value = new Date(Date.UTC(2026, 5, 20 + index));
  return value.toISOString().slice(0, 10);
}

export const medicationClinicalReportInput: MedicationClinicalReportInput = {
  rangeMode: "custom",
  fromDate: "2026-06-20",
  toDate: "2026-07-21",
  zoneId: "America/New_York",
  dayStartHour: 18,
  includeForecast: false,
  includeMedication: true,
  includeMedicationLabels: false,
  includeMedicationNotes: false,
  includeRhythmContext: true,
  includeRhythmContextNotes: false,
};

export function medicationClinicalReportFixture(rowCount = 32): MedicationClinicalReport {
  const count = Math.max(3, rowCount);
  const rows = Array.from({ length: count }, (_, index) => {
    const noData = index === count - 1;
    const annotations = [];
    if (index === 0) {
      annotations.push(
        {
          kind: "medication_taken" as const,
          positionPercent: 25,
          label: "Medication 1 taken",
          atLabel: "12:00 AM",
        },
        {
          kind: "medication_start" as const,
          positionPercent: 30,
          label: "Medication 1 recorded start",
          atLabel: "1:12 AM",
        },
      );
    }
    if (index === 1) {
      annotations.push(
        {
          kind: "medication_skipped" as const,
          positionPercent: 20,
          label: "Medication 1 skipped",
          atLabel: "10:48 PM",
        },
        {
          kind: "context_forced_schedule" as const,
          positionPercent: 40,
          label: "Forced schedule context",
          atLabel: "3:36 AM",
        },
      );
    }
    return {
      civilDate: civilDate(index),
      dayLabel: new Intl.DateTimeFormat("en-US", {
        month: "short",
        day: "numeric",
        timeZone: "UTC",
      }).format(new Date(`${civilDate(index)}T12:00:00Z`)),
      ...(index === 0
        ? { monthLabel: "June 2026" }
        : civilDate(index).endsWith("-01")
          ? { monthLabel: "July 2026" }
          : {}),
      weekend: index % 7 > 4,
      noData,
      sleep: noData
        ? []
        : [
            {
              kind: "sleep_observed" as const,
              startPercent: 16.67,
              widthPercent: 33.33,
              startLabel: "10:00 PM",
              wakeLabel: "6:00 AM",
              durationLabel: "8 hr",
              source: "User-recorded sleep",
              confidence: "High",
            },
          ],
      annotations,
    };
  });

  return {
    status: "partial",
    message: `1 of ${count} calendar rows have no recorded sleep.`,
    generatedAt: "2026-07-22T14:00:00Z",
    generatedLabel: "Jul 22, 2026, 10:00 AM",
    range: {
      mode: "custom",
      fromDate: civilDate(0),
      toDate: civilDate(count - 1),
      label: `${civilDate(0)} to ${civilDate(count - 1)}`,
      dayStartHour: 18,
      dayStartLabel: "6:00 PM to 6:00 PM next day",
    },
    summary: {
      calendarRows: count,
      observedSleepSegments: count - 1,
      noDataRows: 1,
      medicationEvents: 2,
      recordedScheduled: 2,
      recordedTaken: 1,
      recordedSkipped: 1,
      excludedEvents: 1,
      rhythmContextMarkers: 1,
    },
    redactions: [
      "Personal diagnostic information omitted",
      "Calendar and location information omitted",
      "Clinician-entered medication guidance omitted",
      "Medication labels, forms, and strength labels replaced with neutral aliases",
      "Medication notes omitted",
      "Private rhythm-context notes omitted",
      "Forecast bands omitted",
    ],
    actogram: {
      axisLabels: ["6 PM", "12 AM", "6 AM", "12 PM", "6 PM"],
      rows,
      legend: [
        { kind: "sleep_observed", label: "Recorded sleep" },
        { kind: "medication_taken", label: "Recorded taken event" },
        { kind: "medication_skipped", label: "Recorded skipped event" },
        { kind: "medication_start", label: "User-recorded medication start" },
        { kind: "context_forced_schedule", label: "Self-reported forced schedule" },
      ],
      summary:
        "Single-plot clinical actogram. Each row is one civil day; forecast is included only when explicitly selected.",
    },
    drift: {
      status: "estimated",
      slopeLabel: "+42 min per cycle",
      confidence: "Medium",
      summary: "Observed sleep onsets trend later across the selected records.",
      yMinHour: 20,
      yMaxHour: 25,
      points: [
        {
          id: "sleep_01",
          day: "Jun 20",
          civilDate: civilDate(0),
          onsetHour: 22,
          fitHour: 22.1,
          bandLowHour: 21.8,
          bandHighHour: 22.4,
          onsetLabel: "10:00 PM",
          source: "observed",
          confidence: "High",
        },
        {
          id: "sleep_02",
          day: "Jun 21",
          civilDate: civilDate(1),
          onsetHour: 22.7,
          fitHour: 22.8,
          bandLowHour: 22.5,
          bandHighHour: 23.1,
          onsetLabel: "10:42 PM",
          source: "observed",
          confidence: "High",
        },
      ],
    },
    adherence: [
      {
        medicationLabel: "Medication 1",
        recordedScheduled: 2,
        taken: 1,
        skipped: 1,
        asNeeded: 0,
        summary:
          "1 of 2 explicitly recorded scheduled events were marked taken; 1 was marked skipped. No unlogged dose is counted as missed.",
      },
    ],
    events: [
      {
        medicationLabel: "Medication 1",
        civilTime: "Jun 20, 2026, 12:00 AM EDT",
        status: "taken",
        scheduleContext: "Recorded scheduled event",
        wakeContext: "8 hr after recorded wake",
        sleepContext: "2 hr before recorded sleep",
        confidence: "High",
      },
      {
        medicationLabel: "Medication 1",
        civilTime: "Jun 21, 2026, 10:48 PM EDT",
        status: "skipped",
        scheduleContext: "Recorded scheduled event",
        wakeContext: "7 hr after recorded wake",
        sleepContext: "1 hr before recorded sleep",
        confidence: "High",
      },
    ],
    associations: [
      {
        medicationLabel: "Medication 1",
        startedLabel: "Jun 20, 2026, 1:12 AM EDT",
        status: "available",
        message:
          "The recorded start aligns with a change in descriptive drift; temporal alignment does not establish cause.",
        before: {
          episodeCount: 7,
          rangeLabel: "Jun 6 to Jun 19",
          slopeLabel: "+50 min per cycle",
          confidence: "Medium",
        },
        after: {
          episodeCount: 7,
          rangeLabel: "Jun 20 to Jul 3",
          slopeLabel: "+10 min per cycle",
          confidence: "Medium",
        },
        context: [
          {
            kindLabel: "Forced schedule",
            rangeLabel: "Jun 20 to Jun 22",
            timingLabel: "began on the recorded start date",
          },
        ],
      },
    ],
    provenance: [
      "Sleep bands use effective local sleep observations after append-only corrections; gaps are not filled.",
      "Medication rows use effective user-recorded taken or skipped events; excluded events are counted but omitted.",
      "Start comparisons use robust descriptive sleep-onset slopes on each side of a user-recorded medication start.",
    ],
    notice:
      "This report summarizes recorded sleep, medication events, and self-reported context. It does not diagnose or establish treatment effects.",
  };
}

export function medicationClinicalReportExportFixture(): MedicationClinicalReportExport {
  return {
    fileName: "zeitboard-clinician-report-2026-06-20-to-2026-07-21.html",
    html: `<!doctype html><html><head><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'"><title>Report</title></head><body>Local report</body></html>`,
    generatedAt: "2026-07-22T14:00:00Z",
    generatedLabel: "Jul 22, 2026, 10:00 AM",
    rowCount: 32,
    eventCount: 2,
    redactions: [
      "Personal diagnostic information omitted",
      "Calendar and location information omitted",
      "Clinician-entered medication guidance omitted",
    ],
  };
}
