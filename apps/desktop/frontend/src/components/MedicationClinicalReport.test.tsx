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

    expect(await screen.findByText("Adherence summary")).toBeInTheDocument();
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
});
