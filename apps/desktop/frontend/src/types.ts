// Five primary destinations, then a utility group (slice U-H). Eight
// equal-weight destinations was too much undifferentiated navigation for
// someone operating under fatigue, which is the condition this product is for.
export type ScreenId = "home" | "plan" | "rhythm" | "log" | "sharing" | "data-sources" | "settings";

export type PlanTab = "tasks" | "week";
export type LogTab = "sleep" | "medications" | "context";
export type SettingsTab = "display" | "reaching" | "sync" | "computer" | "data";
export type RhythmTab = "actogram" | "drift" | "sources";
