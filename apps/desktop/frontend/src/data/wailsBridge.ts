export type WailsMethod = (input?: unknown) => Promise<unknown>;

export interface WailsRoot {
  go?: unknown;
}

// A missing method in a desktop build is a service failure, not preview mode.
export function hasDesktopBridge(root: WailsRoot = globalThis as WailsRoot): boolean {
  return root.go !== undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

export function findWailsMethod(
  root: WailsRoot,
  names: readonly string[],
): WailsMethod | undefined {
  const packages = root.go;
  if (!isRecord(packages)) return undefined;

  for (const packageValue of Object.values(packages)) {
    if (!isRecord(packageValue)) continue;
    for (const serviceValue of Object.values(packageValue)) {
      if (!isRecord(serviceValue)) continue;
      for (const name of names) {
        const candidate = serviceValue[name];
        if (typeof candidate === "function") {
          return (candidate as WailsMethod).bind(serviceValue);
        }
      }
    }
  }

  return undefined;
}
