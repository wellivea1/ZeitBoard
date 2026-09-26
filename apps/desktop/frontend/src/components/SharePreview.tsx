import { useLayoutEffect, useRef } from "react";
import { Loading } from "./Loading";
import {
  loadPortalPreview,
  sanitizePortalMarkup,
  sanitizePortalStylesheet,
  type PortalPreview,
} from "../data/portalPreview";
import { useLoaded } from "../state/useLoaded";

// The recipient's page inside this window, isolated in a shadow root so its
// stylesheet and this app's cannot touch each other. It is drawn from the
// server's own rendering of the page, never a copy of it, which is the point:
// the owner sees exactly what was shared.
function PortalPage({ preview }: { preview: PortalPreview }) {
  const host = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const element = host.current;
    if (!element) return;
    const shadow = element.shadowRoot ?? element.attachShadow({ mode: "open" });
    const style = document.createElement("style");
    style.textContent = sanitizePortalStylesheet(preview.stylesheet);
    const page = document.createElement("div");
    page.className = "portal-root";
    page.appendChild(sanitizePortalMarkup(preview.html, document));
    shadow.replaceChildren(style, page);
  }, [preview]);
  return <div className="share-preview-page" ref={host} />;
}

export function SharePreview({ profileId, label }: { profileId: string; label: string }) {
  const { data: preview } = useLoaded(() => loadPortalPreview(profileId));
  return (
    <figure className="share-preview" aria-label={`What ${label} sees`}>
      <figcaption>
        What <strong>{label}</strong> sees now, after entering the passcode. Nothing here is
        recorded as a visit.
      </figcaption>
      {preview === undefined ? (
        <Loading>Asking your server for the page…</Loading>
      ) : preview.status === "ok" ? (
        <PortalPage preview={preview} />
      ) : (
        <p className="form-error" role="alert">
          {preview.message ?? "The preview could not be shown."}
        </p>
      )}
    </figure>
  );
}
