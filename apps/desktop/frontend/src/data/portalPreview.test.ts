import { describe, expect, it } from "vitest";
import { loadPortalPreview, sanitizePortalMarkup, sanitizePortalStylesheet } from "./portalPreview";

function markup(html: string): string {
  const host = document.createElement("div");
  host.appendChild(sanitizePortalMarkup(html, document));
  return host.innerHTML;
}

describe("portal preview sanitizing", () => {
  it("keeps the page the template draws", () => {
    const page =
      '<div class="page"><header class="masthead">Availability</header><main>' +
      '<section class="now" aria-labelledby="state-title"><h1 class="lead" id="state-title">Likely awake now</h1></section>' +
      '<figure class="days"><div aria-hidden="true"><svg viewBox="0 0 240 16" preserveAspectRatio="none" focusable="false">' +
      '<rect class="awake" x="10" y="3" width="60" height="10"></rect><line class="now" x1="5" x2="5" y1="0" y2="16"></line>' +
      "</svg></div></figure></main></div>";
    const result = markup(page);
    expect(result).toContain('<h1 class="lead" id="state-title">Likely awake now</h1>');
    expect(result).toContain('viewBox="0 0 240 16"');
    expect(result).toContain('<rect class="awake" x="10" y="3" width="60" height="10"></rect>');
  });

  it("drops anything that could run, load, style or navigate", () => {
    const result = markup(
      '<p class="lead" style="color:red" onclick="alert(1)">Hi</p>' +
        '<img src="x" onerror="alert(1)"><script>alert(1)</script><style>p{}</style>' +
        '<iframe src="https://example.test"></iframe><form action="/p/x"><input name="a"></form>' +
        '<a class="button" href="/p/token/requests">Ask for a time</a>' +
        '<svg><a href="javascript:alert(1)"><text>t</text></a><use href="#x"></use></svg>',
    );
    for (const banned of [
      "style=",
      "onclick",
      "onerror",
      "<img",
      "<script",
      "<style",
      "<iframe",
      "<form",
      "<input",
      "href",
      "<use",
      "javascript:",
    ]) {
      expect(result).not.toContain(banned);
    }
    // A link keeps its words and loses its destination.
    expect(result).toContain("Ask for a time");
    expect(result).toContain('<p class="lead">Hi</p>');
  });

  it("strips every way the stylesheet could fetch something", () => {
    const css = sanitizePortalStylesheet(
      '@import url("https://example.test/x.css");\n' +
        '@font-face { font-family: "Newsreader"; src: url("/p/assets/fonts/n.woff2") format("woff2"); }\n' +
        ":root, :host { --ink: #221f1a; }\n.day { background: url(https://example.test/pixel.png); }",
    );
    expect(css).not.toContain("url(");
    expect(css).not.toContain("@import");
    expect(css).not.toContain("@font-face");
    expect(css).toContain(":root, :host { --ink: #221f1a; }");
  });
});

describe("loading a preview", () => {
  it("keeps markup only from an ok answer", async () => {
    const root = {
      go: {
        main: {
          App: {
            PreviewBackendShareLink: async () => ({
              status: "error",
              message: "Your server returned a preview for a different link.",
              html: "<p>stale</p>",
            }),
          },
        },
      },
    };
    const preview = await loadPortalPreview("prof_mum", root);
    expect(preview.status).toBe("error");
    expect(preview.html).toBe("");
  });
});
