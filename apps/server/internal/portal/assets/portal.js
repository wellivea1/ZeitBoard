// Keeps an open availability page current (portal-design sections 1 and 3).
//
// Without this file the page reloads itself every five minutes. With it, the
// page learns of a new estimate as soon as the server does, through an event
// stream that carries only a version and its freshness, and changes its words
// when a window opens or closes, at the instant the server said it would. If
// the stream cannot be opened the page asks once a minute instead. It never
// reads anything but its own page, with the visitor's own session.
(function () {
  "use strict";
  var body = document.body;
  if (!body || !body.hasAttribute("data-live")) return;

  var page = location.pathname;
  var seen = null;
  var timer = null;
  var polling = null;
  var longest = 24 * 60 * 60 * 1000;

  function schedule(at) {
    if (timer) clearTimeout(timer);
    var when = Date.parse(at || "");
    if (!isFinite(when)) return;
    // A second after the boundary, so the server agrees that it has passed.
    var delay = Math.min(longest, Math.max(1000, when - Date.now() + 1000));
    timer = setTimeout(refresh, delay);
  }

  function refresh() {
    fetch(page, { credentials: "same-origin", headers: { "X-Portal-Refresh": "1" } })
      .then(function (response) {
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
        current.replaceWith(document.importNode(replacement, true));
        schedule(next.body.getAttribute("data-refresh-at"));
      })
      .catch(function () {
        // The next event, boundary or poll tries again.
      });
  }

  function poll() {
    if (!polling) polling = setInterval(refresh, 60 * 1000);
  }

  schedule(body.getAttribute("data-refresh-at"));
  if (!("EventSource" in window)) {
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
  events.onerror = function () {
    // A dropped stream reconnects by itself. A refused one (too many pages
    // open on this link) stays closed, and the page asks once a minute.
    if (events.readyState === EventSource.CLOSED) poll();
  };
})();
