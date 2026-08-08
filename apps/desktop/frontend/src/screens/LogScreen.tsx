import { PageHeader } from "../components/AppShell";
import { RhythmMarkersPanel } from "../components/RhythmMarkersPanel";
import { ScreenTabPanel, ScreenTabs, type ScreenTab } from "../components/ScreenTabs";
import { SleepLogPanel } from "../components/SleepLogPanel";
import { useRhythmMarkers } from "../state/rhythmMarkers";
import { MedicationsScreen } from "./MedicationsScreen";
import type { LogTab } from "../types";

// Log is everything the user records: sleep and wake, doses, and the context
// that explains an odd night (slice U-H).
//
// They were spread across Data Sources, Medications and a tab on Rhythm, which
// meant three different places to go depending on what had just happened. The
// act is the same act — writing down what occurred — and the estimator reads
// all three the same way.

const logTabs: ScreenTab<LogTab>[] = [
  { id: "sleep", label: "Sleep" },
  { id: "medications", label: "Medications" },
  { id: "markers", label: "Context" },
];

export function LogScreen({ tab, onSelect }: { tab: LogTab; onSelect: (tab: LogTab) => void }) {
  const markers = useRhythmMarkers();

  return (
    <>
      <PageHeader
        title="Log"
        description="Record what happened: sleep and wake, doses taken or skipped, and the context behind an unusual day."
      />
      <section className="screen-tabbed" aria-label="Log">
        <ScreenTabs name="log" label="Log views" tabs={logTabs} active={tab} onSelect={onSelect} />

        <ScreenTabPanel name="log" id="sleep" active={tab}>
          <SleepLogPanel />
        </ScreenTabPanel>

        <ScreenTabPanel name="log" id="medications" active={tab}>
          <MedicationsScreen embedded />
        </ScreenTabPanel>

        <ScreenTabPanel name="log" id="markers" active={tab}>
          <RhythmMarkersPanel
            data={markers.data}
            busy={markers.busy}
            exporting={markers.exporting}
            error={markers.error}
            announcement={markers.announcement}
            onAdd={markers.append}
            onDelete={markers.erase}
            onExport={markers.exportAll}
          />
        </ScreenTabPanel>
      </section>
    </>
  );
}
