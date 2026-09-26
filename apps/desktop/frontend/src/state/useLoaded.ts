import { useEffect, useRef, useState } from "react";
import { createCoalescedRefresh, type CoalescedRefresh } from "../utils/coalescedRefresh";
import { subscribeProjectionRefresh } from "../utils/projectionRefresh";

// One way for a view to read something from the desktop and keep it current.
//
// `data` is undefined until the first read settles, and a view shows that as
// loading. A store that has not answered yet is neither empty nor unavailable,
// and saying either would be false: "No sleep entries yet" over a list that
// is about to fill, or "could not be read" over a chart that is about to draw.

export interface LoadedOptions<T> {
  /** Window events after which the value is read again. */
  events?: readonly string[];
  /**
   * The value makes claims about the present that expire with time, so it is
   * also read again on focus, on becoming visible, and every minute while
   * visible.
   */
  expires?: boolean;
  /** Turns a failed read into a value the view can show. Without it a failed
   * read leaves the last value in place. */
  fallback?: (reason: unknown) => T;
}

export interface Loaded<T> {
  /** Undefined while the first read is in flight. */
  data: T | undefined;
  /** Accepts a value a mutation returned; a read already in flight is
   * superseded so it cannot overwrite the newer value when it lands. */
  set: (value: T) => void;
  /** Reads again now. */
  reload: () => void;
}

export function useLoaded<T>(load: () => Promise<T>, options: LoadedOptions<T> = {}): Loaded<T> {
  const [data, setData] = useState<T | undefined>(undefined);
  const loadRef = useRef(load);
  const fallbackRef = useRef(options.fallback);
  const refreshRef = useRef<CoalescedRefresh | null>(null);
  useEffect(() => {
    loadRef.current = load;
    fallbackRef.current = options.fallback;
  });

  const eventKey = (options.events ?? []).join(" ");
  const expires = options.expires ?? false;

  useEffect(() => {
    const refresh = createCoalescedRefresh(
      () => loadRef.current(),
      (value: T) => setData(value),
      (reason) => {
        const fallback = fallbackRef.current;
        if (fallback) setData(fallback(reason));
      },
    );
    refreshRef.current = refresh;
    const request = () => refresh.request();
    const events = eventKey ? eventKey.split(" ") : [];
    let unsubscribe: () => void;
    if (expires) {
      unsubscribe = subscribeProjectionRefresh(request, events);
    } else {
      for (const name of events) window.addEventListener(name, request);
      request();
      unsubscribe = () => {
        for (const name of events) window.removeEventListener(name, request);
      };
    }
    return () => {
      unsubscribe();
      refresh.dispose();
      refreshRef.current = null;
    };
  }, [eventKey, expires]);

  const [actions] = useState(() => ({
    set: (value: T) => {
      refreshRef.current?.supersede();
      setData(value);
    },
    reload: () => refreshRef.current?.request(),
  }));

  return { data, set: actions.set, reload: actions.reload };
}
