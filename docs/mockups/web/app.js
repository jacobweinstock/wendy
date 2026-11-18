// Core of the UX redesign mockup: helpers, routing, Home, Executions, the
// execution page, the command palette and the mockup menu. The catalog
// screens and forms are in screens.js.
"use strict";

const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const esc = s => String(s ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const enc = encodeURIComponent;
const ART = "static/artwork/";
let after = [];

// Status words, glyphs and classes.
const glyph = s => (ST[s] || ST.waiting)[0];
const word = s => (ST[s] || ST.waiting)[1];
const cls = s => (ST[s] || ST.waiting)[2];
const chip = (s, text) => `<span class="chip ${cls(s)}">${glyph(s)} ${esc(text || word(s))}</span>`;

function dur(sec) {
  sec = Math.round(sec);
  if (sec < 60) return sec + "s";
  const m = Math.floor(sec / 60), s = sec % 60;
  if (m < 60) return m + "m" + String(s).padStart(2, "0");
  return Math.floor(m / 60) + "h" + String(m % 60).padStart(2, "0");
}
function ago(min) {
  if (min < 60) return min + " min ago";
  if (min < 1440) return Math.round(min / 60) + " h ago";
  return Math.round(min / 1440) + " d ago";
}
function hash(s) { let h = 7; for (const c of s) h = (h * 31 + c.charCodeAt(0)) >>> 0; return h; }
const dsec = key => 20 + (hash(key) % 280);
const origin = o => (o === "built-in" ? "Built-in" : o);
const short = u => u.replace(/^https:\/\/github.com\//, "");

function toast(msg) {
  const t = $("#toast");
  t.textContent = msg;
  t.hidden = false;
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => (t.hidden = true), 2600);
}

// Model helpers.
const partOf = (tb, kind) => D.testbeds[tb].parts[kind] || "default";
const setting = (tb, k) => D.testbeds[tb][k] || SETTINGS[k].d;
const optLabel = (def, v) => (def.o.find(x => x[0] === v) || [v, v])[1];
const pval = (kind, name, path) => ((D.parts[kind][name] || { v: {} }).v)[path];
const execById = id => D.execs.find(e => e.id === id);

function partSummary(kind, name) {
  const v = (D.parts[kind][name] || { v: {} }).v;
  switch (kind) {
    case "tinkerbell":
      return v["source.repo"] ? "Built from " + v["source.repo"] + (v["source.ref"] ? "@" + v["source.ref"] : "") : "Chart " + (v["chart.version"] || "latest");
    case "clusterAPI":
      return "Cluster API " + (v.core || "v1.14.4") + " · CAPT " + (v["capt.source.repo"] ? "built from " + short(v["capt.source.repo"]) + "@" + (v["capt.source.ref"] || "HEAD") : v["capt.version"] || "v0.7.0");
    case "managementCluster":
      return v.image ? "kind · " + v.image : "kind · default node image (Kubernetes v1.35.0)";
    case "workloadCluster":
      return "Kubernetes " + (v.kubernetesVersion || "v1.36.4") + " · " + (v.cni === "kube-router" ? "kube-router" : "Cilium");
    case "hardware": {
      const m = r => v["machines." + r + ".count"] ?? 1;
      return (v.arch || "amd64") + " · " + m("controlPlane") + " control plane, " + m("worker") + (m("worker") === 1 ? " worker, " : " workers, ") + m("spare") + " spare";
    }
  }
  return "";
}

function combosOf(tbName) {
  const m = D.testbeds[tbName].matrix;
  if (!m) return [{ name: tbName, picks: [] }];
  let out = [{ name: "", picks: [] }];
  for (const ax of m.order) {
    const next = [];
    for (const c of out) {
      for (const [val, patch] of Object.entries(m.axes[ax])) {
        next.push({ name: c.name ? c.name + "-" + val : val, picks: [...c.picks, [ax, val, patch]] });
      }
    }
    out = next;
  }
  return out;
}
const comboNames = e => (e.combos ? Object.keys(e.combos) : [D.plans[e.plan].testbed]);

function usedByPart(kind, name) {
  return Object.keys(D.testbeds).filter(t => {
    if (partOf(t, kind) === name) return true;
    const m = D.testbeds[t].matrix;
    return m && Object.values(m.axes).some(ax => Object.values(ax).some(p => p[0] === kind && p[1] === name));
  });
}
const usedByTestbed = n => Object.keys(D.plans).filter(p => D.plans[p].testbed === n);
const usedBySuite = n => Object.keys(D.plans).filter(p => D.plans[p].suites.includes(n));
const usedByTest = n => Object.keys(D.suites).filter(s => D.suites[s].tests.includes(n));

function buildsOf(tbName) {
  const out = [];
  const t = partOf(tbName, "tinkerbell"), repo = pval("tinkerbell", t, "source.repo");
  if (repo) out.push(["tinkerbell", "Tinkerbell from " + repo]);
  const c = partOf(tbName, "clusterAPI"), crepo = pval("clusterAPI", c, "capt.source.repo");
  if (crepo) out.push(["capt", "CAPT from " + short(crepo) + "@" + (pval("clusterAPI", c, "capt.source.ref") || "HEAD")]);
  return out;
}
const testSteps = t => D.tests[t].steps.map(s => (Array.isArray(s) ? s[0] : s.name || s.kind));

// walk lays out one combination of an execution as tree rows, with each
// step's status derived from the execution's marker.
function walk(e, comboName) {
  const plan = D.plans[e.plan], tbName = plan.testbed;
  const c = e.combos ? e.combos[comboName] : e;
  const mark = c.mark;
  const live = e.status === "running" || e.status === "paused";
  const bad = mark && (mark.st === "failed" || mark.st === "errored");
  let state = mark === "waiting" ? "after" : "before";
  const rows = [];
  const step = (depth, label, key) => {
    let st;
    if (state === "before") {
      if (mark && mark.at === key) { st = mark.st; state = "after"; } else st = "passed";
    } else st = live ? "waiting" : "skipped";
    const secs = st === "running" ? dsec(key) / 2 : ["passed", "failed", "errored"].includes(st) ? dsec(key) : 0;
    rows.push({ depth, label, key, st, secs });
  };
  const group = (depth, label, key) => rows.push({ depth, label, key, st: null, secs: 0, group: true });

  const builds = buildsOf(tbName);
  if (builds.length) { group(0, "Build images", "build"); builds.forEach(([id, l]) => step(1, l, "build/" + id)); }
  group(0, "Set up testbed", "setup");
  SETUP_STEPS.forEach(s => step(1, s, "setup/" + s));
  for (const su of plan.suites) {
    group(0, su, su);
    for (const t of D.suites[su].tests) {
      group(1, t, su + "/" + t);
      for (const s of testSteps(t)) step(2, s, su + "/" + t + "/" + s);
    }
  }
  const ranAll = !live && state === "before";
  if (bad || plan.diagnostics === "always") {
    group(0, "Collect diagnostics", "diag");
    DIAG_STEPS.forEach(s => {
      const st = live ? "waiting" : bad || ranAll ? "passed" : "skipped";
      rows.push({ depth: 1, label: s, key: "diag/" + s, st, secs: st === "passed" ? dsec(s) / 4 : 0 });
    });
  }
  const tst = live ? "waiting" : e.kept ? "skipped" : "passed";
  rows.push({ depth: 0, label: e.kept && !live ? "Tear down (testbed kept)" : "Tear down", key: "teardown", st: tst, secs: tst === "passed" ? 48 : 0 });

  for (let i = rows.length - 1; i >= 0; i--) {
    const r = rows[i];
    if (!r.group) continue;
    const kids = [];
    for (let j = i + 1; j < rows.length && rows[j].depth > r.depth; j++) if (rows[j].depth === r.depth + 1) kids.push(rows[j]);
    r.st = agg(kids.map(k => k.st), live);
    r.secs = kids.reduce((a, k) => a + k.secs, 0);
  }
  return rows;
}

function agg(sts, live) {
  for (const s of ["failed", "errored", "running"]) if (sts.includes(s)) return s;
  if (sts.every(s => s === "passed")) return "passed";
  if (sts.every(s => s === "skipped")) return "skipped";
  if (sts.every(s => s === "waiting")) return "waiting";
  if (sts.includes("passed") && sts.includes("waiting")) return live ? "running" : "waiting";
  return sts.includes("passed") ? "passed" : "skipped";
}

function execStats(e) {
  const names = comboNames(e);
  const trees = Object.fromEntries(names.map(n => [n, walk(e, n)]));
  const leaves = names.flatMap(n => trees[n].filter(r => !r.group));
  const count = s => leaves.filter(r => r.st === s).length;
  const done = leaves.filter(r => ["passed", "failed", "errored", "skipped"].includes(r.st)).length;
  const comboSt = n => agg(trees[n].filter(r => r.depth === 0 && r.key !== "teardown").map(r => r.st), e.status === "running");
  return { names, trees, leaves, count, done, total: leaves.length, comboSt };
}

// Routing.
function route() {
  const h = location.hash.replace(/^#\/?/, "");
  const [path, qs] = h.split("?");
  return { seg: path.split("/").filter(Boolean).map(decodeURIComponent), q: new URLSearchParams(qs || "") };
}

function render() {
  const { seg, q } = route();
  let tab = "", html;
  switch (seg[0] || "") {
    case "": tab = "home"; html = D.firstLaunch ? pWelcome() : pHome(); break;
    case "executions": tab = "executions"; html = seg[1] ? pExecution(+seg[1], q) : pExecutions(q); break;
    case "run": html = pRun(seg[1], q); break;
    case "plans": tab = "plans"; html = pPlans(seg[1]); break;
    case "testbeds": tab = "testbeds"; html = pTestbeds(seg[1] || "testbeds", seg[2]); break;
    case "tests": tab = "tests"; html = pTests(seg[1] || "suites", seg[2]); break;
    case "edit": tab = ["suites", "tests"].includes(seg[1]) ? "tests" : "testbeds"; html = pEdit(seg[1], seg[2], q); break;
    case "config": html = pConfig(); break;
    case "glossary": html = pGlossary(); break;
    default: html = `<div class="page empty"><p>This screen is not in the mockup.</p></div>`;
  }
  $("#top").innerHTML = topbar(tab);
  $("#main").innerHTML = html;
  const fns = after;
  after = [];
  fns.forEach(f => f());
}

function topbar(tab) {
  const t = (id, label, href) => `<a class="tab${tab === id ? " on" : ""}" href="${href}"${tab === id ? ' aria-current="page"' : ""}>${label}</a>`;
  const running = D.execs.filter(e => e.status === "running").length;
  return `<a class="logo" href="#/" title="Wendy: end-to-end tests for Tinkerbell and CAPT">
      <img class="logo-for-light" src="${ART}Tinkerbell-Logo-Landscape-Dark-2.png" alt="Tinkerbell">
      <img class="logo-for-dark" src="${ART}Tinkerbell-Logo-Landscape-Light-2.png" alt="Tinkerbell"></a>
    ${t("home", "Home", "#/")}${t("executions", "Executions", "#/executions")}${t("plans", "Plans", "#/plans")}${t("testbeds", "Testbeds", "#/testbeds")}${t("tests", "Tests", "#/tests")}
    <span class="sp"></span>
    ${D.configError ? `<a class="chip fail" href="#/config" style="text-decoration:none;margin-right:10px">⚠ 1 config error</a>` : ""}
    ${running ? `<a class="chip run" href="#/executions?status=running" style="text-decoration:none;margin-right:10px">● ${running} running</a>` : ""}
    <button type="button" class="themebtn" id="themetoggle" aria-label="Toggle dark mode" title="Toggle dark mode">
      <svg class="sun" viewBox="0 0 20 20" aria-hidden="true"><circle cx="10" cy="10" r="4"></circle><path d="M10 1v3M10 16v3M1 10h3M16 10h3M3.6 3.6l2.1 2.1M14.3 14.3l2.1 2.1M3.6 16.4l2.1-2.1M14.3 5.7l2.1-2.1" stroke="currentColor" stroke-width="1.6"></path></svg>
      <svg class="moon" viewBox="0 0 20 20" aria-hidden="true"><path d="M17.293 13.293A8 8 0 016.707 2.707a8.001 8.001 0 1010.586 10.586z"></path></svg>
    </button>
    <a class="btn primary" href="#/run">Run tests…</a>`;
}

// Home.
function planRow(name, primary) {
  const p = D.plans[name], last = p.last && execById(p.last);
  return `<div class="profile">
    <div class="col"><a class="name" href="#/plans/${enc(name)}" style="text-decoration:none">${esc(name)}</a>
      <div class="desc">${esc(p.desc)}</div></div>
    ${last ? `<a class="chip ${cls(last.status)}" style="text-decoration:none" href="#/executions/${last.id}" title="Last execution #${last.id}">${glyph(last.status)} ${last.status === "running" ? "running" : dur(last.secs)}</a>` : `<span class="chip skip">Not run yet</span>`}
    <button class="btn${primary ? " primary" : ""}" data-toast="Mockup: this would run ${esc(name)} as execution #413">Run</button>
    <a class="btn" href="#/run/${enc(name)}">Change and run…</a>
  </div>`;
}

function keptLine(e) {
  const tb = D.plans[e.plan].testbed;
  return `<div class="row" style="align-items:center;gap:8px;margin-top:8px">
    <span class="${cls(e.status)}">${glyph(e.status)}</span>
    <div class="col"><a href="#/executions/${e.id}"><b>${e.id}/${esc(tb)}</b></a>
      <div class="mute" style="font-size:12px">${esc(e.plan)} · kept ${ago(e.ago)}</div></div>
    <button class="btn sm" data-copy="export KUBECONFIG=~/.local/state/wendy/executions/${e.id}/${esc(tb)}/kubeconfigs/workload">Copy export KUBECONFIG</button>
    <button class="btn sm danger" data-confirm="Tear down ${e.id}/${esc(tb)}? This deletes its clusters, VMs and network." data-toast="Tearing down ${e.id}/${esc(tb)}…">Tear down…</button>
  </div>`;
}

function pHome() {
  const paused = D.execs.filter(e => e.status === "paused");
  const running = D.execs.filter(e => e.status === "running");
  const kept = D.execs.filter(e => e.kept);
  const recent = D.execs.filter(e => !["running", "paused"].includes(e.status)).slice(0, 5);
  return `<div class="page">
    ${paused.map(e => `<div class="banner">
        <span class="pause" style="font-size:20px">‖</span>
        <div class="col"><b>Waiting for you:</b> <a href="#/executions/${e.id}">#${e.id} ${esc(e.plan)}</a> paused after a failure. The testbed is kept so you can look around.
          <div class="mute" style="font-size:13px">${esc(D.failures[e.id].title)}</div></div>
        <div class="actions" style="margin:0">
          <a class="btn primary" href="#/executions/${e.id}">Open execution</a>
          <button class="btn" data-toast="Mockup: retrying capt.scale">Retry step</button>
          <button class="btn" data-toast="Mockup: recording the failure and running the rest" title="Record the failure and run the rest">Continue</button>
          <button class="btn danger" data-confirm="Abort #${e.id} and tear down its testbed? This deletes its clusters, VMs and network." data-toast="Mockup: aborting #${e.id}">Abort and tear down</button>
        </div></div>`).join("")}
    <div class="row">
      <div class="col" style="flex:1.15">
        <div class="card"><h3>Plans</h3>
          ${Object.keys(D.plans).map((n, i) => planRow(n, i === 0)).join("")}
          <a class="profile" style="border-style:dashed;text-decoration:none" href="#/run">
            <span class="name">Run tests…</span><span class="desc">Pick a testbed, suites and options</span></a>
        </div>
      </div>
      <div class="col">
        <div class="card"><h3>Running</h3>
          ${running.length ? "" : `<div class="mute">Nothing is running.</div>`}
          ${running.map(e => { const s = execStats(e); const cur = s.leaves.find(r => r.st === "running"); return `
            <div class="row" style="align-items:center;gap:8px;margin-top:6px">
              <span class="run">●</span><a href="#/executions/${e.id}"><b>#${e.id}</b></a><span>${esc(e.plan)}</span>
              <span class="mute">${s.names.length} combinations</span><span style="flex:1"></span><span class="mono">${dur(e.secs)}</span></div>
            <div class="bar" style="margin:8px 0 2px"><i style="width:${Math.round(100 * s.done / s.total)}%"></i></div>
            <div class="mute" style="font-size:12px">${s.done}/${s.total} steps · now ${esc(cur ? cur.key.split("/").join(" › ") : "")} · ${esc(e.eta)}</div>`; }).join("")}
        </div>
        ${kept.length ? `<div class="card"><h3>Kept testbeds</h3>
          <div class="mute" style="font-size:12px">Left up after an execution. They use this host's memory and disk until you tear them down.</div>
          ${kept.map(keptLine).join("")}</div>` : ""}
        <div class="card"><h3>Recent</h3><table>
          ${recent.map(e => `<tr><td class="${cls(e.status)}">${glyph(e.status)}</td><td><a href="#/executions/${e.id}">#${e.id}</a></td>
            <td>${esc(e.plan)} <span class="mute">${resultCounts(e)}</span></td><td class="mono">${dur(e.secs)}</td><td class="mute">${ago(e.ago)}</td></tr>`).join("")}
        </table><div class="actions"><a class="btn sm" href="#/executions">All executions</a></div></div>
      </div>
    </div></div>`;
}

function resultCounts(e) {
  if (!e.combos) return "";
  const s = execStats(e);
  const by = {};
  s.names.forEach(n => { const st = s.comboSt(n); by[st] = (by[st] || 0) + 1; });
  return Object.entries(by).map(([st, n]) => n + " " + glyph(st)).join(" · ");
}

function pWelcome() {
  return `<div class="page"><div class="empty">
    <h2 style="margin:0 0 6px">Welcome to Wendy</h2>
    <p class="mute">No config yet. Wendy looked in <span class="mono">~/.config/wendy</span> and <span class="mono">./.wendy</span>.</p>
    <p>You can run the built-in plan right now:</p>
    <div class="profile" style="max-width:760px;margin:12px auto 8px;text-align:left">
      <span class="name">quickstart</span><span class="desc">${esc(D.plans.quickstart.desc)} About 14 min.</span>
      <button class="btn primary" data-toast="Mockup: this would run quickstart as execution #1">Run</button>
    </div>
    <p>or <button class="btn sm" data-act="starter">Create ./.wendy/wendy.cue</button>
      <span class="mute">a commented starter file; Wendy reloads it as you edit</span></p>
    <p style="margin-top:22px"><b>Host checks</b>
      &nbsp;<span class="pass">✓ docker</span> &nbsp;<span class="pass">✓ libvirt</span> &nbsp;<span class="pass">✓ /dev/kvm</span>
      &nbsp;<span class="fail">✗ git</span> <span class="mute">(only needed to build from source)</span></p>
  </div></div>`;
}

// Executions.
const EXEC_FILTERS = [["", "All"], ["running", "Running"], ["paused", "Waiting for you"], ["passed", "Passed"], ["failed", "Failed"], ["errored", "Errored"], ["kept", "Kept testbeds"]];

function execResult(e) {
  if (e.status === "paused") return `<span class="chip fail">✗ failed</span> <span class="chip pause">‖ paused</span>`;
  if (e.status === "running") { const s = execStats(e); return `<span class="chip run">● ${s.done}/${s.total} steps</span>`; }
  return chip(e.status) + ` <span class="mute">${resultCounts(e)}</span>`;
}

function pExecutions(q) {
  const f = q.get("status") || "";
  const list = D.execs.filter(e => !f || (f === "kept" ? e.kept : e.status === f));
  return `<div class="page">
    <div class="row" style="align-items:center;margin-bottom:10px">
      <input id="filter" class="input" type="search" placeholder="Filter by number, plan or testbed" aria-label="Filter executions">
      <span class="seg">${EXEC_FILTERS.map(([v, l]) => `<a class="${f === v ? "on" : ""}" href="#/executions${v ? "?status=" + v : ""}">${l}</a>`).join("")}</span>
    </div>
    ${list.length ? `<table>
      <tr><th>Execution</th><th>Plan</th><th>Result</th><th>Testbed</th><th>Started</th><th>Took</th><th></th></tr>
      ${list.map(e => { const tb = D.plans[e.plan].testbed; return `<tr data-f="${e.id} ${esc(e.plan)} ${esc(tb)}">
        <td><a href="#/executions/${e.id}">#${e.id}</a></td>
        <td><a href="#/plans/${enc(e.plan)}">${esc(e.plan)}</a></td>
        <td>${execResult(e)}</td>
        <td><a href="#/testbeds/testbeds/${enc(tb)}">${esc(tb)}</a>${e.combos ? ` <span class="mute">× ${e.combos ? Object.keys(e.combos).length : 1}</span>` : ""}${e.kept ? ` <a class="chip pause" style="text-decoration:none" href="#/executions/${e.id}" title="The testbed is still up">kept</a>` : ""}</td>
        <td>${ago(e.ago)}</td><td class="mono">${dur(e.secs)}</td>
        <td style="white-space:nowrap">${e.status === "running"
          ? `<button class="btn sm danger" data-confirm="Cancel #${e.id}? Its testbeds are torn down unless kept." data-toast="Mockup: cancelling #${e.id}">Cancel execution</button>`
          : `<button class="btn sm" data-toast="Mockup: running ${esc(e.plan)} again as #413">Run again</button>
             <button class="btn sm" data-confirm="Delete #${e.id} and its logs and artifacts?" data-toast="Mockup: deleted #${e.id}">Delete</button>`}</td>
      </tr>`; }).join("")}
    </table>` : `<div class="empty"><p>No executions match.</p><a class="btn" href="#/executions">Show all executions</a></div>`}
  </div>`;
}

const PHASE = { build: "Build images", setup: "Set up testbed", diag: "Collect diagnostics", teardown: "Tear down" };

function pExecution(id, q) {
  const e = execById(id);
  if (!e) return `<div class="page empty"><p>No execution #${id}.</p></div>`;
  const plan = D.plans[e.plan], tb = plan.testbed;
  const s = execStats(e);
  const multi = s.names.length > 1;
  const view = q.get("view") || (multi ? "grid" : "tree");
  const markName = s.names.find(n => { const c = e.combos ? e.combos[n] : e; return c.mark && c.mark.st && c.mark.st !== "running"; });
  const sel = q.get("c") || markName || s.names[0];
  const live = e.status === "running" || e.status === "paused";
  const failedCombos = multi ? s.names.filter(n => ["failed", "errored"].includes(s.comboSt(n))).length : 0;
  const url = extra => "#/executions/" + id + "?" + new URLSearchParams(Object.assign({ view, c: sel }, extra)).toString();
  const head = `<div class="row" style="align-items:center;margin-bottom:14px;gap:12px;flex-wrap:wrap">
      ${e.status === "paused" ? `<span class="chip pause">‖ paused after a failure</span>` : chip(e.status)}
      <span class="mono">${dur(e.secs)}</span>
      <span>plan <a href="#/plans/${enc(e.plan)}">${esc(e.plan)}</a> · testbed <a href="#/testbeds/testbeds/${enc(tb)}">${esc(tb)}</a>${multi ? ` · ${s.names.length} combinations` : ""}</span>
      <div class="col" style="min-width:120px"><div class="bar${s.count("failed") + s.count("errored") ? " failed" : ""}"><i style="width:${Math.round(100 * s.done / s.total)}%"></i></div></div>
      <span>${s.done}/${s.total} steps</span>
      <span class="pass">✓ ${s.count("passed")}</span><span class="fail">✗ ${s.count("failed")}</span><span class="err">! ${s.count("errored")}</span>
      ${e.eta ? `<span class="mute">${esc(e.eta)}</span>` : ""}
      ${multi ? `<span class="seg"><a class="${view === "grid" ? "on" : ""}" href="${url({ view: "grid" })}">Grid</a><a class="${view === "tree" ? "on" : ""}" href="${url({ view: "tree" })}">Tree</a></span>` : ""}
      ${e.status === "running"
        ? `<button class="btn danger sm" data-confirm="Cancel #${id}? Its testbeds are torn down unless kept." data-toast="Mockup: cancelling #${id}">Cancel execution</button>`
        : `${failedCombos ? `<button class="btn" data-toast="Mockup: running the failed combinations again">Run failed again (${failedCombos})</button>` : ""}
           ${e.status !== "paused" ? `<button class="btn" data-toast="Mockup: running ${esc(e.plan)} again as #413">Run again</button>` : ""}
           <button class="btn" data-toast="Mockup: downloads report.junit.xml">JUnit report</button>`}
    </div>`;
  return `<div class="crumbs"><a href="#/executions">Executions</a> › <b>#${id} ${esc(e.plan)}</b></div>
    <div class="page">${head}
      ${D.failures[id] ? failCard(e, markName) : ""}
      ${e.kept ? keptCard(e, markName || sel) : ""}
      ${view === "grid" ? gridView(e, s, url) : treeView(e, s, sel, q.get("node"), url, multi)}
    </div>`;
}

function failCard(e, comboName) {
  const f = D.failures[e.id];
  const paused = e.status === "paused";
  return `<div class="failcard${f.errored ? " errored" : ""}">
    <h3>${esc(f.title)}</h3>
    ${e.combos ? `<div class="mute">combination ${esc(comboName)}</div>` : ""}
    <div style="white-space:pre-wrap">${esc(f.msg)}</div>
    ${f.related ? `<div style="margin-top:4px"><b>Related:</b> ${esc(f.related)}</div>` : ""}
    <div style="margin-top:10px"><span class="mute">${paused ? "Collected so far:" : "Diagnostics:"}</span>
      ${(paused ? ["Console: worker-2", "Workflow log", "BMC log"] : ["Cluster dump", "Tinkerbell objects", "BMC log", "Console: worker-1"])
        .map(l => `<button class="btn sm" data-toast="Mockup: opens ${esc(l)}">${esc(l)}</button>`).join(" ")}</div>
    ${paused ? `<div class="actions">
        <button class="btn primary" data-toast="Mockup: retrying capt.scale">Retry step</button>
        <button class="btn" data-toast="Mockup: recording the failure and running the rest">Continue <span class="mute">(record the failure, run the rest)</span></button>
        <button class="btn danger" data-confirm="Abort #${e.id} and tear down its testbed? This deletes its clusters, VMs and network." data-toast="Mockup: aborting #${e.id}">Abort and tear down</button></div>`
      : f.errored ? `<div class="mute" style="margin-top:8px">Errored means Wendy or the host failed, not Tinkerbell or CAPT. Running it again usually helps.</div>` : ""}
  </div>`;
}

function keptCard(e, comboName) {
  const addr = e.id + "/" + comboName;
  const kube = c => `~/.local/state/wendy/executions/${e.id}/${comboName}/kubeconfigs/${c}`;
  return `<div class="card" style="border-color:var(--pause);margin-bottom:14px">
    <h3>Kept testbed <span class="mono" style="text-transform:none;letter-spacing:0">${esc(addr)}</span></h3>
    <table class="kv">
      <tr><th>Clusters</th><td>management <span class="mono">kind wendy-${e.id}</span> · workload <span class="mono">capt-playground</span></td></tr>
      <tr><th>Tinkerbell</th><td class="mono">172.20.0.100 · ${esc(partSummary("tinkerbell", partOf(D.plans[e.plan].testbed, "tinkerbell")))}</td></tr>
      <tr><th>Machines</th><td class="mono">cp-1 control plane 172.20.0.11 · worker-1 172.20.0.21 · worker-2 172.20.0.22</td></tr>
      <tr><th>BMC</th><td class="mono">Redfish 172.20.0.2:8000</td></tr>
      <tr><th>Network</th><td class="mono">wendy-${e.id} · 172.20.0.0/24 · IPv4</td></tr>
    </table>
    <div class="actions">
      <button class="btn" data-copy="export KUBECONFIG=${kube("workload")}">Copy export KUBECONFIG</button>
      <button class="btn" data-toast="Mockup: downloads the management kubeconfig">management kubeconfig</button>
      <button class="btn" data-toast="Mockup: downloads the workload kubeconfig">workload kubeconfig</button>
      <button class="btn danger" data-confirm="Tear down ${esc(addr)}? This deletes its clusters, VMs and network." data-toast="Mockup: tearing down ${esc(addr)}">Tear down…</button>
    </div>
  </div>`;
}

function gridView(e, s, url) {
  const keys = s.trees[s.names[0]].filter(r => r.depth === 0).map(r => r.key);
  return `<table class="grid">
    <tr><th>Combination</th>${keys.map(k => `<th style="text-align:center">${esc(PHASE[k] || k)}</th>`).join("")}<th>Time</th><th></th></tr>
    ${s.names.map(n => { const rows = s.trees[n]; const c = e.combos ? e.combos[n] : e; const f = c.mark && c.mark.st && c.mark.st !== "running" ? D.failures[e.id] : null; return `
      <tr class="${f ? "sel" : ""}"><td><a href="${url({ view: "tree", c: n })}">${esc(n)}</a></td>
        ${keys.map(k => { const r = rows.find(x => x.depth === 0 && x.key === k); return `<td class="c ${cls(r.st)}" title="${esc((PHASE[k] || k) + ": " + word(r.st))}">${glyph(r.st)}</td>`; }).join("")}
        <td class="mono">${dur(rows.filter(r => r.depth === 0).reduce((a, r) => a + r.secs, 0))}</td>
        <td class="${f ? (f.errored ? "err" : "fail") : ""}">${f ? esc(f.title) : ""}</td></tr>`; }).join("")}
  </table>
  <p class="mute" style="font-size:12px;margin-top:10px">${["passed", "failed", "errored", "running", "waiting", "skipped"].map(st => `<span class="${cls(st)}">${glyph(st)} ${word(st)}</span>`).join(" &nbsp; ")}
    &nbsp;·&nbsp; <span class="err">! errored</span> means Wendy or the host failed, not Tinkerbell or CAPT.</p>`;
}

function treeView(e, s, sel, node, url, multi) {
  const rows = s.trees[sel];
  const c = e.combos ? e.combos[sel] : e;
  const pick = rows.find(r => r.key === node) || rows.find(r => c.mark && r.key === c.mark.at) || rows.find(r => !r.group && r.st === "running") || rows[rows.length - 1];
  return `${multi ? `<div style="margin-bottom:10px"><span class="mute" style="font-size:13px">Combination</span> <span class="seg">${s.names.map(n => `<a class="${n === sel ? "on" : ""}" href="${url({ c: n, node: "" })}"><span class="${cls(s.comboSt(n))}">${glyph(s.comboSt(n))}</span> ${esc(n)}</a>`).join("")}</span></div>` : ""}
  <div class="row">
    <div class="card tree" style="width:400px;flex:none">
      ${rows.map(r => `<a href="${url({ node: r.key })}" class="${r === pick ? "sel" : ""}" style="padding-left:${6 + r.depth * 16}px">
        <span class="${cls(r.st)}">${glyph(r.st)}</span><span>${r.group && r.depth === 0 && !PHASE[r.key] ? "suite " : ""}${esc(r.label)}</span>
        ${r.secs ? `<span class="t">${dur(r.secs)}</span>` : ""}</a>`).join("")}
    </div>
    <div class="col card">${nodeDetail(e, pick, sel)}</div>
  </div>`;
}

function nodeDetail(e, r, comboName) {
  const c = e.combos ? e.combos[comboName] : e;
  const failed = c.mark && c.mark.at === r.key && D.failures[e.id];
  const t = "10:4" + (hash(r.key) % 10) + ":";
  let log = "";
  if (r.st === "waiting") log = "";
  else if (failed && e.id === 410) log = [t + "02 scaling MachineDeployment capt-playground-md-0 from 1 to 2", t + "03 claimed spare machine worker-2", t + "05 Tinkerbell workflow capt-playground-worker-2 started", t + "21 action stream-image: still running (16m)", t + "27 waiting for MachineDeployment capt-playground-md-0: 1/2 ready", t + "27 timed out after 25m: worker-2 did not become a Ready node"].join("\n");
  else if (failed) log = [t + "01 starting " + r.label, t + "09 " + D.failures[e.id].msg].join("\n");
  else if (r.group) log = "";
  else log = [t + "01 starting " + r.label, t + "17 " + (r.st === "running" ? "waiting for MachineDeployment capt-playground-md-0: 1/2 ready" : r.st === "skipped" ? "skipped: an earlier step failed" : "done in " + dur(r.secs))].join("\n");
  return `<div class="row" style="align-items:baseline;gap:10px">
      <b>${esc(r.key.split("/").map(k => PHASE[k] || k).join(" › "))}</b><span class="mute">${r.secs ? dur(r.secs) : ""}</span>
      <span style="flex:1"></span><span class="${cls(r.st)}">${glyph(r.st)} ${word(r.st)}</span></div>
    ${failed ? `<div class="fail" style="margin-top:4px">${esc(failed.title)}</div>` : ""}
    ${r.group ? `<p class="mute">Pick a step to see its log.</p>` : log ? `<pre class="log" style="margin-top:10px">${esc(log)}</pre>` : `<p class="mute">Not started yet.</p>`}
    ${!r.group && r.st !== "waiting" ? `<div style="margin-top:10px"><span class="mute">Artifacts:</span>
      <button class="btn sm" data-toast="Mockup: opens step.log">step.log</button>
      ${failed ? `<button class="btn sm" data-toast="Mockup: opens the console">console-worker-2.log</button>` : ""}</div>
    <div class="actions"><button class="btn sm" data-toast="Mockup: opens the full log">Open full log</button>
      ${e.kept || e.status === "running" ? `<button class="btn sm" data-copy="export KUBECONFIG=~/.local/state/wendy/executions/${e.id}/${esc(comboName)}/kubeconfigs/workload">Copy export KUBECONFIG</button>` : ""}</div>` : ""}`;
}

// Command palette: shows only glossary words, but also matches the old ones.
function paletteItems() {
  const items = [];
  const go = (label, hint, href, words = "") => items.push({ label, hint, href, words });
  go("Run tests…", "Start", "#/run", "new run start execute");
  go("Home", "Go to", "#/", "dashboard");
  go("Executions", "Go to", "#/executions", "runs run history results");
  go("Plans", "Go to", "#/plans", "profile profiles saved");
  go("Testbeds", "Go to", "#/testbeds", "environment environments matrix matrices catalog parts");
  go("Tests", "Go to", "#/tests", "scenario scenarios suites steps catalog");
  go("Config files", "Go to", "#/config", "settings errors cue reload");
  for (const p in D.plans) go("Run " + p, "Plan", "#/run/" + enc(p), "start execute profile");
  for (const e of D.execs) go("#" + e.id + " " + e.plan, "Execution · " + word(e.status), "#/executions/" + e.id, "run");
  for (const t in D.testbeds) go(t, "Testbed", "#/testbeds/testbeds/" + enc(t), "environment matrix");
  const alias = { clusterAPI: "providers capi capt", workloadCluster: "workload kubernetes k8s", managementCluster: "kind management" };
  for (const k of PART_KINDS) {
    go(KINDS[k].label + " parts", "Go to", "#/testbeds/" + SLUG[k], alias[k] || "");
    for (const n in D.parts[k]) go(n, KINDS[k].label + " part", "#/testbeds/" + SLUG[k] + "/" + enc(n), alias[k] || "");
  }
  for (const n in D.suites) go(n, "Suite", "#/tests/suites/" + enc(n));
  for (const n in D.tests) go(n, "Test", "#/tests/tests/" + enc(n), "scenario");
  for (const n in D.steps) go(n, "Built-in step", "#/tests/steps/" + enc(n));
  return items;
}

let palSel = 0, palList = [];
function openPalette() { $("#palette").hidden = false; const i = $("#palette-input"); i.value = ""; palSel = 0; drawPalette(); i.focus(); }
function closePalette() { $("#palette").hidden = true; }
function drawPalette() {
  const q = $("#palette-input").value.toLowerCase().split(/\s+/).filter(Boolean);
  palList = paletteItems().filter(it => q.every(w => (it.label + " " + it.hint + " " + it.words).toLowerCase().includes(w))).slice(0, 12);
  palSel = Math.min(palSel, Math.max(palList.length - 1, 0));
  $("#palette-list").innerHTML = palList.map((it, i) => `<div class="it${i === palSel ? " sel" : ""}" data-pal="${i}"><span>${esc(it.label)}</span><span class="mute">${esc(it.hint)}</span></div>`).join("") || `<div class="it mute">Nothing matches.</div>`;
}
function pickPalette(i) { const it = palList[i]; if (!it) return; closePalette(); location.hash = it.href; }

// Mockup menu.
const MOCK = [
  ["Screens"],
  ["Home", "#/"], ["Executions", "#/executions"],
  ["Execution: running, matrix (#412)", "#/executions/412"], ["Execution: paused after a failure (#410)", "#/executions/410"],
  ["Execution: failed, testbed kept (#405)", "#/executions/405"], ["Execution: errored, grid (#409)", "#/executions/409"],
  ["Run tests (blank)", "#/run"], ["Run tests from plan nightly", "#/run/nightly"],
  ["Plans", "#/plans/nightly"],
  ["Testbed with a matrix", "#/testbeds/testbeds/versions"], ["Edit a testbed", "#/edit/testbeds/ipv6"], ["New testbed", "#/edit/testbeds"],
  ["Management cluster part", "#/testbeds/management-cluster/k8s-1.34"], ["Edit a hardware part", "#/edit/hardware/ha"], ["Edit a Cluster API part", "#/edit/cluster-api/capt-main"],
  ["Suite", "#/tests/suites/lifecycle"], ["Edit a suite", "#/edit/suites/mine"], ["Test", "#/tests/tests/scale-out"], ["Edit a test (Chainsaw step)", "#/edit/tests/hardware-objects"], ["Built-in step", "#/tests/steps/capt.scale"],
  ["Config files", "#/config"],
  ["State"],
  ["toggle:firstLaunch", "First launch (no config)"], ["toggle:configError", "A config file has an error"],
  ["Reference"],
  ["Glossary", "#/glossary"],
];
function mockMenu() {
  $("#mock-menu").innerHTML = MOCK.map(([a, b]) => {
    if (!b) return `<div class="h">${a}</div>`;
    if (a.startsWith("toggle:")) { const k = a.slice(7); return `<label><input type="checkbox" data-toggle="${k}"${D[k] ? " checked" : ""}> ${b}</label>`; }
    return `<a href="${b}">${a}</a>`;
  }).join("");
}

// Events.
document.addEventListener("click", ev => {
  const pal = ev.target.closest("[data-pal]");
  if (pal) { pickPalette(+pal.dataset.pal); return; }
  if (ev.target.classList.contains("overlay")) { ev.target.hidden = true; return; }
  if (ev.target.closest("#mock-btn")) { const m = $("#mock-menu"); m.hidden = !m.hidden; return; }
  if (ev.target.closest("#mock-menu a")) $("#mock-menu").hidden = true;
  if (ev.target.closest("#themetoggle")) {
    const dark = document.documentElement.classList.toggle("dark");
    try { localStorage.setItem("theme", dark ? "dark" : "light"); } catch (e) { /* ignore */ }
    return;
  }
  const t = ev.target.closest("[data-confirm],[data-copy],[data-toast],[data-act]");
  if (!t) return;
  if (t.dataset.confirm && !confirm(t.dataset.confirm)) { ev.preventDefault(); return; }
  if (t.dataset.copy) {
    if (navigator.clipboard) navigator.clipboard.writeText(t.dataset.copy).catch(() => {});
    toast("Copied: " + t.dataset.copy);
  }
  if (t.dataset.toast) toast(t.dataset.toast);
  if (t.dataset.act && ACTS[t.dataset.act]) ACTS[t.dataset.act](t, ev);
});

document.addEventListener("change", ev => {
  const tg = ev.target.closest("[data-toggle]");
  if (tg) { D[tg.dataset.toggle] = tg.checked; render(); }
});

document.addEventListener("input", ev => {
  if (ev.target.id === "palette-input") { palSel = 0; drawPalette(); return; }
  if (ev.target.id === "filter") {
    const q = ev.target.value.toLowerCase();
    $$("[data-f]").forEach(el => (el.hidden = q !== "" && !el.dataset.f.toLowerCase().includes(q)));
  }
});

document.addEventListener("keydown", e => {
  if ((e.metaKey || e.ctrlKey) && e.key === "k") { openPalette(); e.preventDefault(); return; }
  if (!$("#palette").hidden) {
    if (e.key === "ArrowDown") { palSel = Math.min(palSel + 1, palList.length - 1); drawPalette(); e.preventDefault(); }
    else if (e.key === "ArrowUp") { palSel = Math.max(palSel - 1, 0); drawPalette(); e.preventDefault(); }
    else if (e.key === "Enter") pickPalette(palSel);
    else if (e.key === "Escape") closePalette();
    return;
  }
  if (e.key === "Escape") { $("#help").hidden = true; $("#mock-menu").hidden = true; if (document.activeElement) document.activeElement.blur(); return; }
  if (/INPUT|TEXTAREA|SELECT/.test(document.activeElement.tagName) || e.metaKey || e.ctrlKey || e.altKey) return;
  if (e.key === ":") { openPalette(); e.preventDefault(); }
  else if (e.key.length === 1 && "12345".includes(e.key)) location.hash = ["#/", "#/executions", "#/plans", "#/testbeds", "#/tests"][+e.key - 1];
  else if (e.key === "r") location.hash = "#/run";
  else if (e.key === "/") { const f = $("#filter"); if (f) { f.focus(); e.preventDefault(); } }
  else if (e.key === "?") $("#help").hidden = !$("#help").hidden;
});

const ACTS = {
  starter() { D.firstLaunch = false; mockMenu(); render(); toast("Mockup: created ./.wendy/wendy.cue"); },
};

window.addEventListener("hashchange", () => { render(); window.scrollTo(0, 0); });
window.addEventListener("load", () => { render(); mockMenu(); });
