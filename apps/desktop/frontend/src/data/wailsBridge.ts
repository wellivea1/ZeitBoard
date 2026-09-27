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

/**
 * A Go error is a lowercase clause without a full stop, by Go convention.
 * Shown to the owner, it reads as a sentence.
 */
export function asSentence(message: string): string {
  const trimmed = message.trim();
  if (!trimmed) return trimmed;
  const capitalized = trimmed.charAt(0).toLocaleUpperCase() + trimmed.slice(1);
  return /[.!?…]$/.test(capitalized) ? capitalized : `${capitalized}.`;
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
          const method = candidate as (...args: unknown[]) => Promise<unknown>;
          // Wails forwards every argument it is handed and serialises an
          // undefined one as null. A Go method that takes no argument then
          // refuses the call, and the promise it returned never settles.
          return async (input?: unknown) => {
            try {
              return await (input === undefined
                ? method.call(serviceValue)
                : method.call(serviceValue, input));
            } catch (reason) {
              // Wails rejects with a Go error's bare message. Everything above
              // the bridge reads a failure as an Error, and a string would fall
              // through to a generic "that did not work"; the message is shown
              // as the sentence it is meant to be.
              throw typeof reason === "string" ? new Error(asSentence(reason)) : reason;
            }
          };
        }
      }
    }
  }

  return undefined;
}
