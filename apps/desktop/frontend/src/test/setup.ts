import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// jsdom lays nothing out, so it has no scrolling; navigation scrolls a new
// screen to its top, which jsdom would otherwise report as unimplemented.
window.scrollTo = () => undefined;

afterEach(cleanup);
