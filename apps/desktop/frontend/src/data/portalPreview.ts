import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// The recipient's page, as the owner's server renders it (portal-design
// section 10). The server's markup is data, not code: before it is shown it is
// rebuilt from an allowlist of the elements and attributes the portal template
// uses, so a link, a script, an inline style or an event handler cannot reach
// this window whatever the server sends. The stylesheet loses every rule that
// could fetch something.

export interface PortalPreview {
  status: "ok" | "off" | "error";
  message?: string;
  state?: string;
  html: string;
  stylesheet: string;
}

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// Elements the portal template draws. An `a` is kept as its text: the owner
// holds no link token, so no link in a preview may go anywhere.
const keptElements = new Set([
  "div",
  "header",
  "main",
  "section",
  "aside",
  "figure",
  "figcaption",
  "h1",
  "h2",
  "h3",
  "p",
  "span",
  "strong",
  "em",
  "small",
  "code",
  "ul",
  "ol",
  "li",
  "dl",
  "dt",
  "dd",
  "br",
  "svg",
  "g",
  "rect",
  "line",
]);
const unwrappedElements = new Set(["a", "label", "time"]);

const keptAttributes = new Set([
  "class",
  "id",
  "role",
  "aria-label",
  "aria-labelledby",
  "aria-hidden",
  "viewbox",
  "preserveaspectratio",
  "focusable",
  "x",
  "y",
  "width",
  "height",
  "x1",
  "x2",
  "y1",
  "y2",
]);

function copyAllowed(source: Node, target: Node, document: Document) {
  for (const child of Array.from(source.childNodes)) {
    if (child.nodeType === Node.TEXT_NODE) {
      target.appendChild(document.createTextNode(child.textContent ?? ""));
      continue;
    }
    if (!(child instanceof Element)) continue;
    const name = child.localName;
    if (unwrappedElements.has(name)) {
      copyAllowed(child, target, document);
      continue;
    }
    if (!keptElements.has(name)) continue;
    const copy =
      child.namespaceURI === "http://www.w3.org/2000/svg"
        ? document.createElementNS("http://www.w3.org/2000/svg", name)
        : document.createElement(name);
    for (const attribute of Array.from(child.attributes)) {
      if (keptAttributes.has(attribute.name.toLowerCase())) {
        copy.setAttribute(attribute.name, attribute.value);
      }
    }
    copyAllowed(child, copy, document);
    target.appendChild(copy);
  }
}

/** Rebuilds the server's markup from the allowlist, as nodes of `document`. */
export function sanitizePortalMarkup(html: string, document: Document): DocumentFragment {
  const parsed = new DOMParser().parseFromString(html, "text/html");
  const fragment = document.createDocumentFragment();
  copyAllowed(parsed.body, fragment, document);
  return fragment;
}

/** The stylesheet without anything that could load or import a resource. */
export function sanitizePortalStylesheet(css: string): string {
  return css
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/@font-face\s*\{[^}]*\}/g, "")
    .replace(/@import[^;]*;/g, "")
    .replace(/url\([^)]*\)/g, "none");
}

export async function loadPortalPreview(
  profileId: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<PortalPreview> {
  const method = findWailsMethod(root, ["PreviewBackendShareLink"]);
  if (!method) {
    return {
      status: "off",
      message: "Previews need the ZeitBoard desktop app.",
      html: "",
      stylesheet: "",
    };
  }
  const value = await method({ profileId });
  if (
    !isRecord(value) ||
    (value.status !== "ok" && value.status !== "off" && value.status !== "error")
  ) {
    return { status: "error", message: "The preview could not be read.", html: "", stylesheet: "" };
  }
  return {
    status: value.status,
    message: typeof value.message === "string" ? value.message : undefined,
    state: typeof value.state === "string" ? value.state : undefined,
    html: value.status === "ok" && typeof value.html === "string" ? value.html : "",
    stylesheet:
      value.status === "ok" && typeof value.stylesheet === "string" ? value.stylesheet : "",
  };
}
