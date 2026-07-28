import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { medicationDataChangedEvent } from "../data/medications";
import { rhythmMarkersChangedEvent } from "../data/rhythmMarkers";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";
import {
  medicationClinicalReportExportFixture,
  medicationClinicalReportFixture,
} from "../test/medicationReportFixture";
import { MedicationClinicalReport } from "./MedicationClinicalReport";
import { MedicationReportDrift, MedicationReportTables } from "./MedicationClinicalReportPreview";

type GlobalWithGo = typeof globalThis & { go?: unknown };

afterEach(() => {
  delete (globalThis as GlobalWithGo).go;
});

describe("MedicationClinicalReport", () => {
  it("renders a paged, accessible real-data preview with forecast off", async () => {
    const getReport = vi.fn(async () => structuredClone(medicationClinicalReportFixture()));
    (globalThis as GlobalWithGo).go = {
      main: {
        App: {
          GetMedicationClinicianReport: getReport,
          ExportMedicationClinicianReport: vi.fn(async () =>
            structuredClone(medicationClinicalReportExportFixture()),
          ),
        },
      },
    };

    render(<MedicationClinicalReport available />);

    expect(screen.queryByText("Adherence summary")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Build report" }));

    expect(
      await screen.findByText("Adherence summary", undefined, { timeout: 5_000 }),
    ).toBeInTheDocument();
    expect(getReport).toHaveBeenCalledWith(
      expect.objectContaining({
        dayStartHour: 18,
        includeForecast: false,
        includeMedicationLabels: false,
        includeMedicationNotes: false,
        includeRhythmContextNotes: false,
      }),
    );
    expect(screen.getByText("Rows 1-31 of 32")).toBeInTheDocument();
    expect(screen.getByText("June 2026")).toBeInTheDocument();
    expect(screen.getByText("July 2026")).toBeInTheDocument();
    expect(
      screen.getByRole("table", { name: /sleep and timing details for rows 1 through 31/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("No causal inference")).toBeInTheDocument();
    expect(
      screen.getByText(/Missing logs are not interpreted as missed doses/),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Past 7 days" }));
    const from = screen.getByLabelText("From") as HTMLInputElement;
    const through = screen.getByLabelText("Through") as HTMLInputElement;
    expect(
      (Date.parse(`${through.value}T00:00:00Z`) - Date.parse(`${from.value}T00:00:00Z`)) /
        86_400_000,
    ).toBe(6);
    expect(screen.getByText("Preview controls changed")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Next 31 days" }));
    expect(screen.getByText("Rows 32-32 of 32")).toBeInTheDocument();
    expect(screen.getByText("July 2026")).toBeInTheDocument();
    expect(
      screen.getByRole("table", { name: /sleep and timing details for rows 32 through 32/i }),
    ).toBeInTheDocument();
  });

  it("marks changed controls stale and gates local HTML export behind exact confirmation", async () => {
    const getReport = vi.fn(async () => structuredClone(medicationClinicalReportFixture()));
    const exportReport = vi.fn(async () =>
      structuredClone(medicationClinicalReportExportFixture()),
    );
    (globalThis as GlobalWithGo).go = {
      main: {
        App: {
          GetMedicationClinicianReport: getReport,
          ExportMedicationClinicianReport: exportReport,
        },
      },
    };

    render(<MedicationClinicalReport available />);
    fireEvent.click(screen.getByRole("button", { name: "Build report" }));
    expect(await screen.findByText("Adherence summary")).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("Include private medication labels and strength text"));
    expect(screen.getByText("Preview controls changed")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Prepare HTML export" })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "Regenerate preview" }));
    await waitFor(() => expect(getReport).toHaveBeenCalledTimes(2));
    expect(getReport).toHaveBeenLastCalledWith(
      expect.objectContaining({ includeMedicationLabels: true }),
    );

    fireEvent.click(screen.getByRole("button", { name: "Prepare HTML export" }));
    const exportRegion = screen.getByRole("region", { name: "Printable HTML export" });
    const createButton = within(exportRegion).getByRole("button", {
      name: "Create HTML report",
    });
    expect(createButton).toBeDisabled();
    fireEvent.change(within(exportRegion).getByLabelText("Type EXPORT to create the file"), {
      target: { value: "export" },
    });
    expect(createButton).toBeDisabled();
    fireEvent.change(within(exportRegion).getByLabelText("Type EXPORT to create the file"), {
      target: { value: "EXPORT" },
    });
    expect(createButton).toBeEnabled();
    fireEvent.click(createButton);

    await waitFor(() => expect(exportReport).toHaveBeenCalledTimes(1));
    expect(exportReport).toHaveBeenCalledWith({
      report: expect.objectContaining({ includeMedicationLabels: true }),
      confirmation: "EXPORT",
    });
  });

  it.each([
    ["medication", medicationDataChangedEvent],
    ["rhythm context", rhythmMarkersChangedEvent],
    ["sleep", sleepDataChangedEvent],
  ])("invalidates a generated preview when %s evidence changes", async (_label, eventName) => {
    (globalThis as GlobalWithGo).go = {
      main: {
        App: {
          GetMedicationClinicianReport: vi.fn(async () =>
            structuredClone(medicationClinicalReportFixture()),
          ),
          ExportMedicationClinicianReport: vi.fn(async () =>
            structuredClone(medicationClinicalReportExportFixture()),
          ),
        },
      },
    };

    render(<MedicationClinicalReport available />);
    fireEvent.click(screen.getByRole("button", { name: "Build report" }));
    expect(await screen.findByText("Adherence summary")).toBeInTheDocument();

    fireEvent(window, new Event(eventName));

    expect(screen.getByText("Preview controls changed")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Prepare HTML export" })).toBeDisabled();
  });

  it("stays explicitly unavailable without both clinician-report methods", () => {
    render(<MedicationClinicalReport available />);

    expect(screen.getByRole("button", { name: "Build report" })).toBeDisabled();
    expect(screen.getByText(/require the current ZeitBoard desktop service/)).toBeInTheDocument();
  });

  it("bounds drift graphics and paginates the complete evidence tables", () => {
    const report = medicationClinicalReportFixture();
    const point = report.drift.points[0]!;
    const medicationEvent = report.events[0]!;
    report.drift.points = Array.from({ length: 205 }, (_, index) => ({
      ...point,
      id: `point_${index + 1}`,
      day: `Day ${index + 1}`,
      civilDate: `Date ${index + 1}`,
      onsetLabel: `Onset ${index + 1}`,
      onsetHour: 20 + (index % 20) / 4,
      fitHour: 20 + (index % 20) / 4,
      bandLowHour: 19.75 + (index % 20) / 4,
      bandHighHour: 20.25 + (index % 20) / 4,
    }));
    report.events = Array.from({ length: 101 }, (_, index) => ({
      ...medicationEvent,
      medicationLabel: `Medication event ${index + 1}`,
      civilTime: `Event time ${index + 1}`,
    }));

    const { container } = render(
      <>
        <MedicationReportDrift report={report} />
        <MedicationReportTables report={report} />
      </>,
    );

    expect(container.querySelectorAll(".clinical-drift-point")).toHaveLength(180);
    expect(screen.getByText("Points 1-100 of 205")).toBeVisible();
    expect(screen.getByText("Onset 1")).toBeInTheDocument();
    expect(screen.queryByText("Onset 101")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Next drift points" }));
    expect(screen.getByText("Points 101-200 of 205")).toBeVisible();
    expect(screen.getByText("Onset 101")).toBeInTheDocument();

    expect(screen.getByText("Events 1-100 of 101")).toBeVisible();
    expect(screen.queryByText("Medication event 101")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Next medication events" }));
    expect(screen.getByText("Events 101-101 of 101")).toBeVisible();
    expect(screen.getByText("Medication event 101")).toBeInTheDocument();
  });

  it("labels an empty drift evidence table without an invalid row range", () => {
    const report = medicationClinicalReportFixture();
    report.drift.points = [];

    render(<MedicationReportDrift report={report} />);

    expect(screen.getByRole("table", { name: "Observed sleep-onset drift points" })).toBeVisible();
    expect(screen.queryByText(/1 through 0/)).not.toBeInTheDocument();
  });
});
