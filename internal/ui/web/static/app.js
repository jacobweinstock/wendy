// wendy web UI: keyboard shortcuts, the command palette, copy buttons and
// the live-update stream. Everything else is htmx.
(function () {
  "use strict";

  // A live page learns it is out of date from one event stream, named by
  // <body data-stream data-events>, and re-renders its [data-refresh] parts.
  // Browsers allow only six connections per server over HTTP/1.1, so streams
  // held by background tabs or by pages kept in the back/forward cache soon
  // stall every other request. The stream is therefore held only while the
  // page is on screen, and the page resyncs when it comes back.
  var stream = { es: null, done: false, last: "" };

  function refresh() {
    document.querySelectorAll("[data-refresh]").forEach(function (el) { htmx.trigger(el, "refresh"); });
  }

  function connect() {
    var b = document.body, url = b && b.dataset.stream;
    if (!url || stream.es || stream.done || document.hidden) return false;
    if (stream.last) {
      var u = new URL(url, location.href);
      u.searchParams.set("after", stream.last);
      url = u.pathname + u.search;
    }
    var es = new EventSource(url);
    (b.dataset.events || "").split(" ").forEach(function (t) {
      if (t) es.addEventListener(t, function (e) { if (e.lastEventId) stream.last = e.lastEventId; refresh(); });
    });
    // Without this the browser would reconnect to a finished stream forever.
    es.addEventListener("end", function () { stream.done = true; disconnect(); refresh(); });
    stream.es = es;
    return true;
  }

  function disconnect() {
    if (stream.es) { stream.es.close(); stream.es = null; }
  }

  document.addEventListener("DOMContentLoaded", connect);
  document.addEventListener("visibilitychange", function () {
    if (document.hidden) disconnect();
    else if (connect()) refresh();
  });
  window.addEventListener("pagehide", disconnect);
  window.addEventListener("pageshow", function (e) { if (e.persisted && connect()) refresh(); });

  function typing(e) {
    var t = e.target;
    return t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable);
  }

  var overlay, input, list, items = [], sel = 0;

  function actions() {
    var el = document.getElementById("palette-data");
    return el ? JSON.parse(el.textContent) : [];
  }

  function fuzzy(q, s) {
    q = q.toLowerCase(); s = s.toLowerCase();
    var i = 0;
    for (var j = 0; j < s.length && i < q.length; j++) { if (s[j] === q[i]) i++; }
    return i === q.length;
  }

  function render() {
    var q = input.value;
    items = actions().filter(function (a) { return fuzzy(q, a.label + " " + (a.aliases || "")); }).slice(0, 8);
    sel = Math.min(sel, Math.max(items.length - 1, 0));
    list.innerHTML = "";
    items.forEach(function (a, i) {
      var d = document.createElement("div");
      d.className = "it" + (i === sel ? " sel" : "");
      d.innerHTML = "<span></span><span class=mute></span>";
      d.firstChild.textContent = a.label;
      d.lastChild.textContent = a.kind;
      d.onclick = function () { run(a); };
      list.appendChild(d);
    });
  }

  function run(a) {
    close();
    if (a.post) {
      fetch(a.post, { method: "POST", headers: { "Content-Type": "application/json" }, body: a.body || "{}" })
        .then(function (r) {
          var to = r.headers.get("Location");
          return r.json().then(function (j) { if (to) location.href = to; else if (j.error) alert(j.error); });
        });
    } else if (a.href) {
      location.href = a.href;
    }
  }

  function open() {
    overlay.hidden = false;
    input.value = "";
    sel = 0;
    render();
    input.focus();
  }

  function close() { overlay.hidden = true; }

  document.addEventListener("DOMContentLoaded", function () {
    overlay = document.getElementById("palette");
    input = document.getElementById("palette-input");
    list = document.getElementById("palette-list");
    if (!overlay) return;
    input.addEventListener("input", function () { sel = 0; render(); });
    input.addEventListener("keydown", function (e) {
      if (e.key === "ArrowDown") { sel = Math.min(sel + 1, items.length - 1); render(); e.preventDefault(); }
      else if (e.key === "ArrowUp") { sel = Math.max(sel - 1, 0); render(); e.preventDefault(); }
      else if (e.key === "Enter" && items[sel]) { run(items[sel]); }
      else if (e.key === "Escape") { close(); }
    });
    overlay.addEventListener("click", function (e) { if (e.target === overlay) close(); });
  });

  document.addEventListener("keydown", function (e) {
    if ((e.metaKey || e.ctrlKey) && e.key === "k") { open(); e.preventDefault(); return; }
    if (typing(e) || e.metaKey || e.ctrlKey || e.altKey) return;
    var tabs = document.querySelectorAll("nav.top a.tab");
    if (e.key === ":") { open(); e.preventDefault(); }
    else if (e.key === "/") { var f = document.getElementById("filter"); if (f) { f.focus(); e.preventDefault(); } }
    else if (e.key === "n") { var rt = document.getElementById("run-tests"); if (rt) location.href = rt.href; }
    else if (e.key === "?") { var h = document.getElementById("help"); if (h) h.hidden = !h.hidden; }
    else if (e.key === "Escape") { var h2 = document.getElementById("help"); if (h2) h2.hidden = true; close(); }
    else if (e.key >= "1" && e.key <= "9" && tabs[+e.key - 1]) { location.href = tabs[+e.key - 1].href; }
    else if (e.key === "f") { var fl = document.getElementById("follow"); if (fl) location.href = fl.href; }
  });

  // copyText uses the async clipboard API, which browsers expose only on
  // secure origins; wendy is usually served over plain http on a LAN address,
  // so it falls back to a selection and execCommand.
  function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) return navigator.clipboard.writeText(text);
    return new Promise(function (resolve, reject) {
      var ta = document.createElement("textarea");
      ta.value = text;
      ta.setAttribute("readonly", "");
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.select();
      var ok = false;
      try { ok = document.execCommand("copy"); } catch (err) { ok = false; }
      document.body.removeChild(ta);
      ok ? resolve() : reject(new Error("copy failed"));
    });
  }

  document.addEventListener("click", function (e) {
    var b = e.target.closest("[data-copy]");
    if (!b) return;
    var t = b.textContent;
    var flash = function (msg) { b.textContent = msg; setTimeout(function () { b.textContent = t; }, 1500); };
    copyText(b.dataset.copy).then(function () { flash("Copied"); }, function () { flash("Copy failed"); });
  });

  // The editor: a step shows the fields of what it runs, can move and be
  // removed, and a suite's test names add themselves to its list.
  document.addEventListener("change", function (e) {
    var sel = e.target.closest(".stepkind");
    if (sel) sel.closest(".step").dataset.kind = sel.value;
  });

  document.addEventListener("click", function (e) {
    var b = e.target.closest("[data-remove], [data-move], [data-append]");
    if (!b) return;
    if (b.hasAttribute("data-append")) {
      var ta = document.querySelector('[name="' + b.dataset.into + '"]');
      if (ta) ta.value = (ta.value.trim() ? ta.value.replace(/\s*$/, "\n") : "") + b.dataset.append;
      return;
    }
    var row = b.closest(".step"), form = row.closest("form");
    if (b.hasAttribute("data-remove")) row.remove();
    else {
      var up = +b.dataset.move < 0, sib = up ? row.previousElementSibling : row.nextElementSibling;
      if (sib) row.parentNode.insertBefore(up ? row : sib, up ? sib : row);
    }
    // What depends on the rows' order, such as a matrix's combinations,
    // listens for changes to the form.
    if (form) form.dispatchEvent(new Event("change", { bubbles: true }));
  });

  // An edit form that has changed asks before the page is left, so a link
  // followed by mistake doesn't lose what was typed. A reply that redirects
  // means the form was saved or deleted; htmx reports it before following it.
  var unsaved = false;
  function edited(e) { if (e.target.closest && e.target.closest("form[data-unsaved]")) unsaved = true; }
  document.addEventListener("input", edited);
  document.addEventListener("change", edited);
  window.addEventListener("beforeunload", function (e) { if (unsaved) { e.preventDefault(); e.returnValue = ""; } });
  document.addEventListener("htmx:beforeOnLoad", function (e) { if (e.detail.xhr.getResponseHeader("HX-Redirect")) unsaved = false; });

  // The testbed form makes a part in a dialog. htmx fills the dialog in, and
  // it opens once filled. It empties when it closes, so it never reopens on a
  // stale form. A saved part's reply has already put the part in its picker.
  document.addEventListener("htmx:afterSwap", function (e) {
    var d = e.detail.target;
    if (d && d.id === "partdialog" && !d.open) d.showModal();
  });
  document.addEventListener("close", function (e) { if (e.target.id === "partdialog") e.target.innerHTML = ""; }, true);
  document.addEventListener("click", function (e) {
    var b = e.target.closest("[data-close-dialog]");
    if (b) b.closest("dialog").close();
  });
  document.addEventListener("partSaved", function (e) {
    var d = e.target.closest("dialog");
    if (d) d.close();
    // The testbed form now picks a part it hasn't saved.
    unsaved = true;
    var sel = document.getElementById("p-" + e.detail.kind);
    if (sel) sel.focus();
  });

  // Dark mode: the class on <html> is set before first paint by an inline
  // script in the page head; this only flips and remembers it. Until the user
  // chooses, the page follows the OS setting.
  document.addEventListener("click", function (e) {
    if (!e.target.closest("#themetoggle")) return;
    var dark = document.documentElement.classList.toggle("dark");
    try { localStorage.setItem("theme", dark ? "dark" : "light"); } catch (_) { /* storage blocked */ }
  });

  var scheme = window.matchMedia("(prefers-color-scheme: dark)");
  scheme.addEventListener("change", function (e) {
    var saved = null;
    try { saved = localStorage.getItem("theme"); } catch (_) { /* storage blocked */ }
    if (!saved) document.documentElement.classList.toggle("dark", e.matches);
  });

  // A rejected save is reported at the top; bring it into view.
  document.addEventListener("htmx:afterSwap", function (e) {
    if (e.detail.target && e.detail.target.id === "form-error") e.detail.target.scrollIntoView({ block: "center" });
  });

  // htmx leaves 4xx/5xx responses unswapped; say what went wrong instead.
  document.addEventListener("htmx:responseError", function (e) {
    var msg = e.detail.xhr.responseText || e.detail.xhr.statusText;
    try { msg = JSON.parse(msg).error || msg; } catch (_) { /* not JSON */ }
    var t = document.createElement("div");
    t.className = "toast";
    t.textContent = msg;
    document.body.appendChild(t);
    setTimeout(function () { t.remove(); }, 6000);
  });
})();
