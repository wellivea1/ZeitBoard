import "@testing-library/jest-dom/vitest";
import { cleanup, configure } from "@testing-library/react";
import { afterEach } from "vitest";

// A screen is loaded when it is first opened. On a busy machine that takes
// longer than the second a query waits by default, and tests that open
// Settings timed out while the whole suite ran beside the Go suites. They wait
// longer here; nothing they assert has changed.
configure({ asyncUtilTimeout: 5000 });

// jsdom lays nothing out, so it has no scrolling; navigation scrolls a new
// screen to its top, which jsdom would otherwise report as unimplemented.
window.scrollTo = () => undefined;

afterEach(cleanup);
