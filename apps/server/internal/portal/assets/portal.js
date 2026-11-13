// Keeps an open availability page current (portal-design sections 1 and 3).
//
// Without this file the page reloads itself every five minutes. With it, the
// page learns of a new estimate as soon as the server does, through an event
// stream that carries only a version and its freshness, and changes its words
// when a window opens or closes, at the instant the server said it would. If
// the stream cannot be opened, or is down, the page asks once a minute
// instead. It never reads anything but its own page, with the visitor's own
// session.
//
// A page left open through an outage must not go on saying what it last knew.
// A refresh that fails is tried again, sooner at first and then once a minute,
// and the page says the connection is lost. Once the instant its claim was
// good until has passed without a fresh answer, the claim itself is withdrawn:
// "Likely awake right now" from before the outage is not something to act on.
(function () {
  "use strict";
  var body = document.body;
  var mode = body ? body.getAttribute("data-live") : null;
  if (!mode) return;

  var page = location.pathname;
  var seen = null;
  var timer = null;
  var polling = null;
  var retry = null;
  var failures = 0;
  var longest = 24 * 60 * 60 * 1000;
  // The claim on the page holds until the instant the server gave for its next
  // change. Without one, it is trusted for as long as the page without this
  // script would have gone before reloading.
  var unattended = 5 * 60 * 1000;
  var goodUntil = validUntil(body.getAttribute("data-refresh-at"));

  function validUntil(at) {
    var when = Date.parse(at || "");
    return isFinite(when) ? when : Date.now() + unattended;
  }

  function schedule(at) {
    if (timer) clearTimeout(timer);
    var when = Date.parse(at || "");
    if (!isFinite(when)) return;
    // A second after the boundary, so the server agrees that it has passed.
    var delay = Math.min(longest, Math.max(1000, when - Date.now() + 1000));
    timer = setTimeout(refresh, delay);
  }

  // The fields a visitor may be part way through: what is typed, and where the
  // cursor is. A refresh replaces the page around them, and used to empty a
  // message that had not been sent.
  function drafts(main) {
    var kept = [];
    var fields = main.querySelectorAll("textarea[name], input[name]");
    for (var i = 0; i < fields.length; i++) {
      var field = fields[i];
      if (field.type === "hidden" || field.type === "submit") continue;
      kept.push({
        tag: field.tagName,
        name: field.name,
        value: field.value,
        focused: document.activeElement === field,
        start: field.selectionStart,
        end: field.selectionEnd,
      });
    }
    return kept;
  }

  function restoreDrafts(main, kept) {
    var fields = main.querySelectorAll("textarea[name], input[name]");
    for (var i = 0; i < fields.length; i++) {
      var field = fields[i];
      for (var j = 0; j < kept.length; j++) {
        var draft = kept[j];
        if (draft.tag !== field.tagName || draft.name !== field.name) continue;
        if (draft.value) field.value = draft.value;
        if (draft.focused) {
          field.focus();
          try {
            field.setSelectionRange(draft.start, draft.end);
          } catch (error) {
            // Not every kind of field has a selection to restore.
          }
        }
      }
    }
  }

  // One line under the page's own, said to assistive technology as it changes.
  function note(text) {
    var main = document.querySelector("main");
    if (!main) return;
    var line = document.getElementById("live-note");
    if (!text) {
      if (line) line.remove();
      return;
    }
    if (!line) {
      line = document.createElement("p");
      line.id = "live-note";
      line.className = "meta is-stale";
      line.setAttribute("role", "status");
      var after = main.querySelector(".meta");
      if (after && after.parentNode) after.parentNode.insertBefore(line, after.nextSibling);
      else main.insertBefore(line, main.firstChild);
    }
    line.textContent = text;
  }

  // What the page says once its claim has outlived the last good answer.
  function withdraw() {
    if (mode === "poll") {
      note("The connection was lost. This page will update when it returns.");
      return;
    }
    body.setAttribute("data-offline", "");
    var lead = document.querySelector("main .lead");
    var detail = document.querySelector("main .lead-detail");
    if (lead) lead.textContent = "This page could not be updated";
    if (detail) {
      detail.textContent =
        "The connection was lost, so what was shown here may no longer be true. It will update when the connection returns.";
    }
    note("");
  }

  function failed() {
    failures += 1;
    if (Date.now() >= goodUntil) withdraw();
    else note("The connection was lost. Trying again.");
    // Sooner at first, then once a minute for as long as the page is open.
    var delay = Math.min(60 * 1000, 5000 * Math.pow(2, failures - 1));
    if (retry) clearTimeout(retry);
    retry = setTimeout(refresh, delay);
  }

  function refresh() {
    if (retry) clearTimeout(retry);
    retry = null;
    fetch(page, { credentials: "same-origin", headers: { "X-Portal-Refresh": "1" } })
      .then(function (response) {
        // Too many requests, or a server in trouble, is an outage to wait
        // out. Any other refusal is the link's own answer, and the page
        // itself says it best.
        if (response.status === 429 || response.status >= 500) throw new Error("later");
        if (!response.ok) {
          location.reload();
          return null;
        }
        return response.text();
      })
      .then(function (html) {
        if (typeof html !== "string") return;
        var next = new DOMParser().parseFromString(html, "text/html");
        var current = document.querySelector("main");
        var replacement = next.querySelector("main");
        if (!current || !replacement) {
          location.reload();
          return;
        }
        var kept = drafts(current);
        var fresh = document.importNode(replacement, true);
        current.replaceWith(fresh);
        restoreDrafts(fresh, kept);
        failures = 0;
        body.removeAttribute("data-offline");
        var at = next.body.getAttribute("data-refresh-at");
        goodUntil = validUntil(at);
        schedule(at);
      })
      .catch(failed);
  }

  function poll() {
    if (!polling) polling = setInterval(refresh, 60 * 1000);
  }

  function stopPolling() {
    if (polling) clearInterval(polling);
    polling = null;
  }

  schedule(body.getAttribute("data-refresh-at"));
  // A request's own page has no stream to follow: its answer and its
  // thread arrive through the ordinary page, so it asks once a minute.
  if (mode === "poll" || !("EventSource" in window)) {
    poll();
    return;
  }
  var events = new EventSource(page + "/events");
  events.addEventListener("state", function (event) {
    var state;
    try {
      state = JSON.parse(event.data);
    } catch (error) {
      return;
    }
    var key = state.version + ":" + state.freshness;
    if (seen !== null && seen !== key) refresh();
    seen = key;
  });
  events.addEventListener("gone", function () {
    events.close();
    location.reload();
  });
  events.onopen = stopPolling;
  events.onerror = function () {
    // A dropped stream tries to reconnect by itself, and may go on trying for
    // as long as the outage lasts; a refused one (too many pages open on this
    // link) stays closed. Either way the page asks once a minute meanwhile.
    poll();
  };
})();
