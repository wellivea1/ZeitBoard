import { useEffect, useRef, useState } from "react";
import {
  addRhythmMarker,
  deleteRhythmMarker,
  downloadRhythmMarkerExport,
  exportRhythmMarkers,
  loadRhythmMarkers,
  notifyRhythmMarkersChanged,
  rhythmMarkersChangedEvent,
  unavailableRhythmMarkers,
  type RhythmMarkerInput,
  type RhythmMarkersData,
} from "../data/rhythmMarkers";
import { createCoalescedRefresh, type CoalescedRefresh } from "../utils/coalescedRefresh";

// Context markers moved from Rhythm to Log in slice U-H: recording that you
// travelled, were ill, or had to be up for an appointment is logging, and it
// belongs beside the sleep and medication records rather than beside the charts
// that interpret them.
//
// The state came with them, and it is here rather than in either screen so a
// second surface can render markers without a second copy of this logic.

export interface RhythmMarkersState {
  data: RhythmMarkersData;
  busy: boolean;
  exporting: boolean;
  error: string;
  announcement: string;
  append: (input: RhythmMarkerInput) => Promise<void>;
  erase: (markerId: string, confirmation: string) => Promise<void>;
  exportAll: () => void;
}

export function useRhythmMarkers(): RhythmMarkersState {
  const [data, setData] = useState(unavailableRhythmMarkers);
  const [busy, setBusy] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [error, setError] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const refreshRef = useRef<CoalescedRefresh | null>(null);

  // A change this hook made itself must not bounce back as a reload: the
  // mutation already returned the new state, and refetching would blank the
  // list for a frame.
  const ignoreNextChange = useRef(false);

  const publishChange = () => {
    ignoreNextChange.current = true;
    notifyRhythmMarkersChanged();
    ignoreNextChange.current = false;
  };

  useEffect(() => {
    const refresh = createCoalescedRefresh(
      loadRhythmMarkers,
      (result) => {
        setData(result);
        setError("");
      },
      (reason) => {
        setError(reason instanceof Error ? reason.message : "Rhythm markers could not be loaded.");
      },
    );
    refreshRef.current = refresh;
    const request = () => {
      if (ignoreNextChange.current) {
        ignoreNextChange.current = false;
        return;
      }
      refresh.request();
    };
    request();
    window.addEventListener(rhythmMarkersChangedEvent, request);
    return () => {
      window.removeEventListener(rhythmMarkersChangedEvent, request);
      if (refreshRef.current === refresh) refreshRef.current = null;
      refresh.dispose();
    };
  }, []);

  const append = async (input: RhythmMarkerInput) => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const result = await addRhythmMarker(input);
      setData(result);
      setAnnouncement("Context marker appended.");
      refreshRef.current?.supersede();
      publishChange();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Context marker could not be saved.");
      throw reason;
    } finally {
      setBusy(false);
    }
  };

  const erase = async (markerId: string, confirmation: string) => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const result = await deleteRhythmMarker(markerId, confirmation);
      setData(result);
      setAnnouncement("Context marker permanently erased.");
      publishChange();
      refreshRef.current?.supersede();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Context marker could not be erased.");
      throw reason;
    } finally {
      setBusy(false);
    }
  };

  const exportAll = () => {
    if (exporting || data.status === "unavailable") return;
    setExporting(true);
    setError("");
    void exportRhythmMarkers().then(
      (result) => {
        setExporting(false);
        const downloaded = downloadRhythmMarkerExport(result);
        setAnnouncement(
          `${result.markerCount} context ${result.markerCount === 1 ? "marker" : "markers"} exported${downloaded ? ` to ${result.fileName}` : "."}`,
        );
      },
      (reason: unknown) => {
        setExporting(false);
        setError(
          reason instanceof Error ? reason.message : "Context markers could not be exported.",
        );
      },
    );
  };

  return { data, busy, exporting, error, announcement, append, erase, exportAll };
}
