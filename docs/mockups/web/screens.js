// Screens of the UX redesign mockup: the Run tests form, Plans, Testbeds
// and parts, Tests, the edit forms, Config and the glossary.
"use strict";

const plural = (n, w) => n + " " + w + (n === 1 ? "" : "s");
const fmtMins = m => (m < 60 ? m + " min" : Math.floor(m / 60) + " h" + (m % 60 ? " " + (m % 60) + " min" : ""));
const notFound = () => `<div class="page empty"><p>Nothing by that name.</p></div>`;
const cueSource = text => `<details style="margin-top:12px"><summary class="btn">View CUE</summary><pre class="log" style="margin-top:8px">${esc(text)}</pre></details>`;
const detailHead = (n, o) => `<div class="row" style="align-items:baseline;gap:10px"><h2 class="title">${esc(n)}</h2><span class="chip skip">${esc(origin(o))}</span></div>`;

function byOrigin(coll) {
  const rank = o => (o === "built-in" ? 0 : 1);
  return Object.keys(coll).map(n => [n, coll[n].origin || "built-in"])
    .sort((a, b) => rank(a[1]) - rank(b[1]) || a[1].localeCompare(b[1]) || a[0].localeCompare(b[0]));
}

function listCard(items, href, sel) {
  let last = null;
  return `<div class="card tree" style="width:260px;flex:none">${items.map(([n, o]) => {
    const g = o !== last ? `<div class="grp">${esc(origin(o))}</div>` : "";
    last = o;
    return g + `<a href="${href(n)}" class="${n === sel ? "sel" : ""}" data-f="${esc(n)}">${esc(n)}</a>`;
  }).join("")}</div>`;
}

function subtabs(tabs, cur, base, newLabel, newHref) {
  return `<div class="subtabs">${tabs.map(([id, l]) => `<a class="${id === cur ? "on" : ""}" href="${base}${id}"${id === cur ? ' aria-current="page"' : ""}>${esc(l)}</a>`).join("")}
    <span style="flex:1"></span>
    ${newLabel ? `<a class="btn sm" href="${newHref}">${esc(newLabel)}</a>&nbsp;` : ""}
    <input id="filter" class="input" style="min-width:180px" type="search" placeholder="Filter" aria-label="Filter this list"></div>`;
}

function labelsInUse() {
  const s = new Set();
  Object.values(D.tests).forEach(t => (t.labels || []).forEach(l => s.add(l)));
  return [...s].sort().map(l => `<span class="mono">${esc(l)}</span>`).join(", ");
}

// labelMatcher parses a label expression (a && !b || (c)) into a predicate
// over a list of labels.
function labelMatcher(src) {
  if (!src) return () => true;
  const toks = src.match(/&&|\|\||!|\(|\)|[A-Za-z0-9_.=:\/-]+|\S/g) || [];
  let i = 0;
  const peek = () => toks[i];
  const or = () => { let a = and(); while (peek() === "||") { i++; const b = and(), x = a; a = l => x(l) || b(l); } return a; };
  const and = () => { let a = not(); while (peek() === "&&") { i++; const b = not(), x = a; a = l => x(l) && b(l); } return a; };
  const not = () => { if (peek() === "!") { i++; const a = not(); return l => !a(l); } return atom(); };
  const atom = () => {
    const t = toks[i++];
    if (t === "(") { const a = or(); if (toks[i++] !== ")") throw new Error("a ( has no matching )"); return a; }
    if (!t || /^(&&|\|\||\))$/.test(t)) throw new Error("a label is missing" + (t ? " before " + t : " at the end"));
    if (!/^[A-Za-z0-9_.=:\/-]+$/.test(t)) throw new Error(t + " is not a label or an operator");
    return l => l.includes(t);
  };
  const f = or();
  if (i < toks.length) throw new Error("unexpected " + toks[i]);
  return f;
}

// Run tests.
function pRun(from, q) {
  const plan = from && D.plans[from];
  const base = plan || { testbed: q.get("testbed") || "default", suites: q.get("suite") ? [q.get("suite")] : ["smoke"], parallel: 1, keep: "never", pause: false, diagnostics: "onFailure", consoles: true, labels: "" };
  after.push(() => {
    const f = $("#runform");
    const changed = ev => { if (ev.target.name === "testbed") drawCombos(); updatePreview(); };
    f.addEventListener("input", changed);
    f.addEventListener("change", changed);
    drawCombos();
    updatePreview();
  });
  const seg = (name, opts, val) => `<span class="seg">${opts.map(([v, l]) => `<label><input type="radio" name="${name}" value="${v}"${v === val ? " checked" : ""}>${l}</label>`).join("")}</span>`;
  return `<div class="crumbs">${plan ? `<a href="#/plans/${enc(from)}">Plans</a> › ${esc(from)} › ` : ""}Run tests</div>
  <form id="runform" class="runform" onsubmit="return false">
    <div class="page row" style="padding-bottom:70px">
      <div class="col">
        <div class="card"><h3>Testbed</h3>
          <div class="field"><label class="k" for="r-tb">Testbed</label><div>
            <select id="r-tb" name="testbed" class="input">${Object.keys(D.testbeds).map(t => `<option${t === base.testbed ? " selected" : ""}>${esc(t)}</option>`).join("")}</select>
            <div class="hint" id="r-tb-hint"></div></div></div>
          <div id="r-combos"></div>
        </div>
        <div class="card"><h3>Tests</h3>
          <div class="field"><label class="k">Suites</label><div>${Object.keys(D.suites).map(s => `<div><label class="check"><input type="checkbox" name="suites" value="${esc(s)}"${base.suites.includes(s) ? " checked" : ""}> <b>${esc(s)}</b></label> <span class="hint">${esc(D.suites[s].desc)}</span></div>`).join("")}</div></div>
          <div class="field"><label class="k" for="r-labels">Only tests with labels</label><div>
            <input id="r-labels" class="input mono" name="labels" value="${esc(base.labels)}">
            <div class="hint" style="max-width:52ch">Empty runs every test. Combine labels with <span class="mono">&amp;&amp;</span> (and), <span class="mono">||</span> (or) and <span class="mono">!</span> (not), e.g. <span class="mono">scale &amp;&amp; !upgrade</span>. Labels in use: ${labelsInUse()}.</div></div></div>
        </div>
        <div class="card"><h3>Options</h3>
          <div class="field"><label class="k" for="r-par">Combinations at once</label><div>
            <input id="r-par" class="input" style="min-width:0;width:80px" type="number" min="1" name="parallel" value="${base.parallel}">
            <div class="hint">Each one needs its own machines.</div></div></div>
          <div class="field"><label class="k">Keep testbed</label><div>${seg("keep", [["never", "Never"], ["failed", "If it fails"], ["always", "Always"]], base.keep)}
            <div class="hint">A kept testbed stays up after the execution so you can look around.</div></div></div>
          <div class="field"><label class="k" for="r-pause">Pause on failure</label><label class="check" style="padding-top:5px"><input id="r-pause" type="checkbox" name="pause"${base.pause ? " checked" : ""}> Wait for me before tearing anything down</label></div>
          <div class="field"><label class="k">Collect diagnostics</label><div>${seg("diagnostics", [["onFailure", "On failure"], ["always", "Always"], ["never", "Never"]], base.diagnostics)}
            <div class="hint">Cluster dump, Tinkerbell objects and BMC logs.</div></div></div>
          <div class="field"><label class="k" for="r-cons">Machine consoles</label><label class="check" style="padding-top:5px"><input id="r-cons" type="checkbox" name="consoles"${base.consoles ? " checked" : ""}> Record each machine's console</label></div>
        </div>
      </div>
      <div class="col"><div id="preview" class="card" style="background:var(--inset)"></div></div>
    </div>
    <div class="savebar">
      <span class="mute" style="margin-right:auto;font-size:12px">${plan ? `Changes apply to this execution only. Plan ${esc(from)} is not changed.` : "This execution only. Your config files are not changed."}</span>
      <a class="btn" href="${plan ? "#/plans/" + enc(from) : "#/"}">Cancel</a>
      <button type="button" id="run-btn" class="btn primary" data-act="runNow">Run</button>
    </div>
  </form>`;
}

function drawCombos() {
  const tb = $("#r-tb").value, t = D.testbeds[tb], m = t.matrix;
  $("#r-tb-hint").textContent = t.desc;
  $("#r-combos").innerHTML = !m
    ? `<div class="field"><label class="k">Combinations</label><div class="hint" style="padding-top:5px">One: this testbed has no matrix.</div></div>`
    : `<div class="field"><label class="k">Combinations</label><div>
        ${m.order.map(ax => `<div class="facet" style="margin:2px 0">${esc(ax)} ${Object.keys(m.axes[ax]).map(v => `<label><input type="checkbox" name="axis" value="${esc(ax + "=" + v)}" checked><span class="chip">${esc(v)}</span></label>`).join("")}</div>`).join("")}
        <div style="margin-top:6px"><input class="input mono" name="combos" aria-label="Combination names"></div>
        <div class="hint">Click a value to leave it out, or type names (* matches anything, e.g. <span class="mono">1.37-*</span>).</div></div></div>`;
}

function updatePreview() {
  const f = $("#runform");
  if (!f) return;
  const fd = new FormData(f);
  const tb = fd.get("testbed"), suites = fd.getAll("suites"), par = Math.max(1, +fd.get("parallel") || 1);
  const axisOn = new Set(fd.getAll("axis")), glob = (fd.get("combos") || "").trim();
  let combos = combosOf(tb);
  const total = combos.length;
  if (D.testbeds[tb].matrix) combos = combos.filter(c => c.picks.every(([ax, v]) => axisOn.has(ax + "=" + v)));
  if (glob) {
    const re = new RegExp("^(" + glob.split(/[\s,]+/).filter(Boolean).map(g => g.replace(/[.+?^${}()|[\]\\]/g, "\\$&").replace(/\*/g, ".*")).join("|") + ")$");
    combos = combos.filter(c => re.test(c.name));
  }
  let match = () => true, err = "";
  try { match = labelMatcher((fd.get("labels") || "").trim()); } catch (x) { err = x.message; }
  const testsOf = s => D.suites[s].tests.filter(t => D.tests[t].once || match(D.tests[t].labels || []));
  const nTests = suites.reduce((a, s) => a + testsOf(s).length, 0);
  const hv = D.parts.hardware[partOf(tb, "hardware")].v;
  const cnt = r => hv["machines." + r + ".count"] ?? 1;
  const vms = cnt("controlPlane") + cnt("worker") + cnt("spare");
  const at = Math.max(1, Math.min(par, combos.length));
  const builds = buildsOf(tb);
  const mins = 9 + (builds.length ? 4 : 0) + suites.reduce((a, s) => a + testsOf(s).reduce((b, t) => b + (D.tests[t].mins || 5), 0), 0);
  const waves = Math.ceil(Math.max(combos.length, 1) / par);
  const disk = vms * at * 10;
  const problems = [err && "Only tests with labels: " + err, !suites.length && "Pick at least one suite.", !combos.length && "No combination matches. Turn an axis value back on or change the names."].filter(Boolean);
  const notes = [];
  if (fd.get("keep") === "always") notes.push("Every combination's testbed stays up after it finishes. Tear them down from Home when you are done.");
  if (fd.get("pause") && par > 1) notes.push("While one combination is paused, its machines stay in use and the next combination waits for them.");
  if (disk > 100) notes.push(`Needs ${disk} GiB of disk in /tmp, but only 96 GiB is free. Lower combinations at once, or pick a hardware part with a bigger disk directory.`);
  $("#preview").innerHTML = `<h3>Preview</h3>
    ${problems.map(p => `<p class="fail" style="margin:4px 0">${esc(p)}</p>`).join("")}
    <p style="margin:0 0 8px"><b>${plural(combos.length, "combination")} × ${plural(nTests, "test")}</b>${total > 1 ? ` <span class="mute">· ${combos.length} of ${total}</span>` : ""}</p>
    <table>${combos.map((c, i) => `<tr><td>${esc(c.name)}</td><td class="mute">${i ? "same tests" : suites.map(s => `<div>${esc(s)}: ${testsOf(s).map(esc).join(" › ")}</div>`).join("")}</td></tr>`).join("")}</table>
    <table class="kv" style="margin-top:10px">
      <tr><th>Needs</th><td>${vms * at} VMs · ${vms * at * 2 + 4 * at} GiB memory · ${disk} GiB disk${at > 1 ? ` <span class="mute">(${at} combinations at once)</span>` : ""}</td></tr>
      <tr><th>Builds from source</th><td>${builds.map(b => esc(b[1])).join(", ") || `<span class="mute">Nothing</span>`}</td></tr>
      <tr><th>Takes about</th><td>${fmtMins(mins * waves)}</td></tr>
      <tr><th>Host checks</th><td><span class="pass">✓ docker</span> <span class="pass">✓ libvirt</span> <span class="pass">✓ /dev/kvm</span>
        ${disk > 100 ? `<span class="fail">✗ disk</span>` : `<span class="pass">✓ disk</span>`} ${builds.length ? `<span class="pass">✓ git</span>` : ""}</td></tr>
    </table>
    ${notes.map(n => `<div class="note">${esc(n)}</div>`).join("")}`;
  $("#run-btn").disabled = problems.length > 0;
}

ACTS.runNow = () => toast("Mockup: this would start execution #413 and open it");

// Plans.
function optionsLine(p) {
  return [
    plural(p.parallel, "combination") + " at once",
    { never: "never keep the testbed", failed: "keep the testbed if it fails", always: "always keep the testbed" }[p.keep],
    p.pause && "pause on failure",
    { onFailure: "collect diagnostics on failure", always: "always collect diagnostics", never: "never collect diagnostics" }[p.diagnostics],
    p.consoles && "record machine consoles",
  ].filter(Boolean).join(" · ");
}

function planCue(n) {
  const p = D.plans[n];
  const lines = [`plans: ${JSON.stringify(n)}: {`, `\tdescription: ${JSON.stringify(p.desc)}`, `\ttestbed:     ${JSON.stringify(p.testbed)}`, `\tsuites:      ${JSON.stringify(p.suites)}`];
  if (p.parallel !== 1) lines.push(`\tparallel:    ${p.parallel}`);
  if (p.keep !== "never") lines.push(`\tkeep:        ${JSON.stringify(p.keep)}`);
  if (p.pause) lines.push(`\tpause:       true`);
  if (p.diagnostics !== "onFailure") lines.push(`\tdiagnostics: ${JSON.stringify(p.diagnostics)}`);
  return lines.concat("}").join("\n");
}

function pPlans(name) {
  const items = byOrigin(D.plans);
  name = name && D.plans[name] ? name : items[0][0];
  const p = D.plans[name], last = p.last && execById(p.last);
  return `<div class="page">
    <div class="subtabs"><span class="mute" style="padding:6px 0;font-size:13px">${plural(items.length, "plan")}</span><span style="flex:1"></span>
      <input id="filter" class="input" style="min-width:180px" type="search" placeholder="Filter" aria-label="Filter plans"></div>
    <p class="lead">${BLURBS.plans}</p>
    <div class="row">${listCard(items, n => "#/plans/" + enc(n), name)}
      <div class="col card">${detailHead(name, p.origin)}
        <p>${esc(p.desc)}</p>
        <table class="kv">
          <tr><th>Testbed</th><td><a href="#/testbeds/testbeds/${enc(p.testbed)}">${esc(p.testbed)}</a> <span class="mute">${D.testbeds[p.testbed].matrix ? plural(combosOf(p.testbed).length, "combination") : "one combination"}</span></td></tr>
          <tr><th>Suites</th><td>${p.suites.map(s => `<a href="#/tests/suites/${enc(s)}">${esc(s)}</a>`).join(", ")}</td></tr>
          <tr><th>Only tests with labels</th><td>${p.labels ? `<span class="mono">${esc(p.labels)}</span>` : `<span class="mute">Every test</span>`}</td></tr>
          <tr><th>Options</th><td>${optionsLine(p)}</td></tr>
          <tr><th>Last execution</th><td>${last ? `<a href="#/executions/${last.id}">#${last.id}</a> ${chip(last.status)} <span class="mute">${ago(last.ago)}</span>` : `<span class="mute">Not run yet</span>`}</td></tr>
        </table>
        <div class="actions">
          <button class="btn primary" data-toast="Mockup: this would run ${esc(name)} as execution #413">Run</button>
          <a class="btn" href="#/run/${enc(name)}">Change and run…</a>
        </div>
        <p class="hint" style="margin-top:12px">Plans are written in CUE. "Change and run" changes one execution, not the plan.</p>
        ${cueSource(planCue(name))}
      </div></div></div>`;
}

// Testbeds and parts.
const TB_TABS = [["testbeds", "Testbeds"]].concat(PART_KINDS.map(k => [SLUG[k], KINDS[k].label]));

function pTestbeds(sub, name) {
  const kind = UNSLUG[sub], isTb = sub === "testbeds";
  const coll = isTb ? D.testbeds : kind && D.parts[kind];
  if (!coll) return notFound();
  const items = byOrigin(coll);
  name = name && coll[name] ? name : items[0][0];
  return `<div class="page">
    ${subtabs(TB_TABS, sub, "#/testbeds/", isTb ? "＋ New testbed" : "＋ New " + KINDS[kind].one, "#/edit/" + sub)}
    <p class="lead">${isTb ? BLURBS.testbeds : KINDS[kind].blurb}</p>
    <div class="row">${listCard(items, n => "#/testbeds/" + sub + "/" + enc(n), name)}
      <div class="col card">${isTb ? testbedDetail(name) : partDetail(kind, name)}</div></div></div>`;
}

function patchText(p) {
  if (KINDS[p[0]]) return `${KINDS[p[0]].label} <a href="#/testbeds/${SLUG[p[0]]}/${enc(p[1])}">${esc(p[1])}</a>`;
  return `${SETTINGS[p[0]].l}: ${esc(optLabel(SETTINGS[p[0]], p[1]))}`;
}

function matrixHtml(n) {
  const m = D.testbeds[n].matrix;
  return `<table>${m.order.map(ax => `<tr><td class="mono">${esc(ax)}</td><td>${Object.entries(m.axes[ax]).map(([v, p]) => `<span class="mono">${esc(v)}</span> → ${patchText(p)}`).join(" &nbsp;·&nbsp; ")}</td></tr>`).join("")}</table>
    <div class="hint" style="margin-top:4px">${plural(combosOf(n).length, "combination")}: ${combosOf(n).map(c => esc(c.name)).join(", ")}</div>`;
}

function networkExtras(t) {
  const fam = t.family || "ipv4";
  return [t.nat64 === true || (t.nat64 === undefined && fam === "ipv6") ? "NAT64 on" : "No NAT64", t.mirror ? "registry mirror on" : "no registry mirror"].join(" · ");
}

function machinesLine(v) {
  return ROLES.map(([r, l]) => {
    const g = c => v["machines." + r + "." + c] ?? MCOLS.find(x => x[0] === c)[2];
    return `${g("count")} ${l.toLowerCase()}: ${g("cpus")} CPUs, ${g("memMiB")} MiB, ${g("diskGiB")} GiB`;
  }).join("<br>");
}

function resolvedTable(n) {
  const t = D.testbeds[n], hw = D.parts.hardware[partOf(n, "hardware")].v, wl = D.parts.workloadCluster[partOf(n, "workloadCluster")].v;
  return `<table class="kv" style="margin-top:8px">
    ${t.matrix ? `<tr><th></th><td class="mute">Before the matrix: each combination changes some of these.</td></tr>` : ""}
    <tr><th>Management cluster</th><td>${esc(partSummary("managementCluster", partOf(n, "managementCluster")))}</td></tr>
    <tr><th>Tinkerbell</th><td>${esc(partSummary("tinkerbell", partOf(n, "tinkerbell")))} · namespace tinkerbell · ${esc(optLabel(SETTINGS.topology, setting(n, "topology")).toLowerCase())} · ${esc(optLabel(SETTINGS.bootMode, setting(n, "bootMode")).toLowerCase())}</td></tr>
    <tr><th>Cluster API</th><td>${esc(partSummary("clusterAPI", partOf(n, "clusterAPI")))}</td></tr>
    <tr><th>Workload cluster</th><td>capt-playground · Kubernetes ${esc(wl.kubernetesVersion || "v1.36.4")} · ${wl.cni === "kube-router" ? "kube-router" : "Cilium"} · Ubuntu 24.04 · kube-vip 1.1.2</td></tr>
    <tr><th>Machines</th><td>${machinesLine(hw)}</td></tr>
    <tr><th>Hardware</th><td>${esc(hw.arch || "amd64")} · libvirt qemu:///system · disks in /tmp · BMC Redfish</td></tr>
    <tr><th>Network</th><td>${esc(optLabel(SETTINGS.family, setting(n, "family")))} · ${networkExtras(t)}</td></tr>
    <tr><th>Setup steps</th><td class="mono">${SETUP_STEPS.join(" › ")}</td></tr>
  </table>`;
}

function testbedCue(n) {
  const t = D.testbeds[n], lines = [`testbeds: ${JSON.stringify(n)}: {`, `\tdescription: ${JSON.stringify(t.desc)}`];
  for (const k of PART_KINDS) if (t.parts[k]) lines.push(`\t${k}: ${JSON.stringify(t.parts[k])}`);
  for (const k of Object.keys(SETTINGS)) if (t[k]) lines.push(`\t${k === "family" ? "network: family" : k}: ${JSON.stringify(t[k])}`);
  if (t.nat64) lines.push(`\tnetwork: nat64: true`);
  if (t.mirror) lines.push(`\tnetwork: mirror: enabled: true`);
  if (t.matrix) {
    lines.push(`\tmatrix: {`, `\t\torder: ${JSON.stringify(t.matrix.order)}`, `\t\taxes: {`);
    for (const ax of t.matrix.order) lines.push(`\t\t\t${ax}: {${Object.entries(t.matrix.axes[ax]).map(([v, p]) => `${JSON.stringify(v)}: ${p[0]}: ${JSON.stringify(p[1])}`).join(", ")}}`);
    lines.push(`\t\t}`, `\t}`);
  }
  return lines.concat("}").join("\n");
}

function testbedDetail(n) {
  const t = D.testbeds[n], used = usedByTestbed(n), editable = t.origin !== "built-in";
  const labels = [setting(n, "topology"), setting(n, "bootMode"), setting(n, "family")];
  return `${detailHead(n, t.origin)}<p>${esc(t.desc)}</p>
    <table class="kv">
      <tr><th>Parts</th><td>${PART_KINDS.map(k => `<div class="partrow"><span class="kind">${KINDS[k].label}</span><a href="#/testbeds/${SLUG[k]}/${enc(partOf(n, k))}">${esc(partOf(n, k))}</a><span class="hint">${esc(partSummary(k, partOf(n, k)))}</span></div>`).join("")}</td></tr>
      ${Object.keys(SETTINGS).map(k => `<tr><th>${SETTINGS[k].l}</th><td>${esc(optLabel(SETTINGS[k], setting(n, k)))}</td></tr>`).join("")}
      <tr><th>Network extras</th><td>${networkExtras(t)}</td></tr>
      <tr><th>Matrix</th><td>${t.matrix ? matrixHtml(n) : `<span class="mute">None: one combination.</span>`}</td></tr>
      <tr><th>Labels</th><td>${labels.map(l => `<span class="chip mono">${esc(l)}</span>`).join(" ")}${t.matrix ? ` <span class="hint">and each combination's axis values, e.g. <span class="mono">k8s=1.36</span></span>` : ""}</td></tr>
      <tr><th>Used by</th><td>${used.length ? used.map(p => `plan <a href="#/plans/${enc(p)}">${esc(p)}</a>`).join(" · ") : `<span class="mute">No plan yet</span>`}</td></tr>
    </table>
    <details style="margin-top:12px"><summary class="btn">Resolved values</summary>${resolvedTable(n)}</details>
    <div class="actions">
      <a class="btn primary" href="#/run?testbed=${enc(n)}">Run tests on this testbed…</a>
      ${editable ? `<a class="btn" href="#/edit/testbeds/${enc(n)}">Edit testbed</a>` : `<a class="btn" href="#/edit/testbeds/${enc(n)}?copy=1" title="Copies it into ./.wendy/wendy-ui.cue, where you can change it">Copy and edit…</a>`}
    </div>
    ${cueSource(testbedCue(n))}`;
}

function partCue(kind, n) {
  const v = D.parts[kind][n].v;
  const keys = Object.keys(v);
  return `parts: ${kind}: ${JSON.stringify(n)}: {${keys.length ? "\n" + keys.map(k => `\t${k.split(".").join(": ")}: ${JSON.stringify(v[k])}`).join("\n") + "\n" : ""}}`;
}

function partDetail(kind, n) {
  const p = D.parts[kind][n], used = usedByPart(kind, n), editable = p.origin !== "built-in";
  const fs = KINDS[kind].fields.filter(f => f.t !== "machines");
  const common = fs.filter(f => !f.adv), adv = fs.filter(f => f.adv), advSet = adv.filter(f => p.v[f.p] !== undefined);
  const val = f => (p.v[f.p] !== undefined
    ? esc(f.t === "opts" ? optLabel(f, p.v[f.p]) : p.v[f.p])
    : `<span class="mute">${esc(f.t === "opts" ? optLabel(f, f.d) : f.d || "Not set")} (default)</span>`);
  return `${detailHead(n, p.origin)}<p class="mute" style="margin-top:4px">${esc(partSummary(kind, n))}</p>
    <table class="kv">
      ${common.map(f => `<tr><th>${esc(f.l)}</th><td>${val(f)}</td></tr>`).join("")}
      ${kind === "hardware" ? `<tr><th>Machines</th><td>${machinesLine(p.v)}</td></tr>` : ""}
      ${advSet.map(f => `<tr><th>${esc(f.l)}</th><td>${val(f)}</td></tr>`).join("")}
      ${adv.length - advSet.length ? `<tr><th></th><td class="mute">${plural(adv.length - advSet.length, "advanced setting")} at the default</td></tr>` : ""}
      <tr><th>Used by</th><td>${used.length ? used.map(t => `testbed <a href="#/testbeds/testbeds/${enc(t)}">${esc(t)}</a>`).join(" · ") : `<span class="mute">No testbed yet</span>`}</td></tr>
    </table>
    <div class="actions">${editable
      ? `<a class="btn" href="#/edit/${SLUG[kind]}/${enc(n)}">Edit ${esc(KINDS[kind].one)}</a>`
      : `<a class="btn" href="#/edit/${SLUG[kind]}/${enc(n)}?copy=1" title="Copies it into ./.wendy/wendy-ui.cue, where you can change it">Copy and edit…</a>`}</div>
    ${cueSource(partCue(kind, n))}`;
}

// Tests.
const TEST_TABS = [["suites", "Suites"], ["tests", "Tests"], ["steps", "Built-in steps"]];

function pTests(sub, name) {
  const coll = { suites: D.suites, tests: D.tests, steps: D.steps }[sub];
  if (!coll) return notFound();
  const items = sub === "steps" ? Object.keys(D.steps).map(n => [n, "built-in"]) : byOrigin(coll);
  name = name && coll[name] ? name : items[0][0];
  const blurb = { suites: BLURBS.suites, tests: BLURBS.tests, steps: BLURBS.steps }[sub];
  const nw = { suites: ["＋ New suite", "#/edit/suites"], tests: ["＋ New test", "#/edit/tests"], steps: ["", ""] }[sub];
  const detail = sub === "suites" ? suiteDetail(name) : sub === "tests" ? testDetail(name) : stepDetail(name);
  return `<div class="page">${subtabs(TEST_TABS, sub, "#/tests/", nw[0], nw[1])}
    <p class="lead">${blurb}</p>
    <div class="row">${listCard(items, n => "#/tests/" + sub + "/" + enc(n), name)}<div class="col card">${detail}</div></div></div>`;
}

function suiteDetail(n) {
  const s = D.suites[n], used = usedBySuite(n), editable = s.origin !== "built-in";
  return `${detailHead(n, s.origin)}<p>${esc(s.desc)}</p>
    <table class="kv">
      <tr><th>Tests, in order</th><td>${s.tests.map((t, i) => `<div>${i + 1}. <a href="#/tests/tests/${enc(t)}">${esc(t)}</a> <span class="hint">${esc(D.tests[t].desc)}</span></div>`).join("")}</td></tr>
      <tr><th>Used by</th><td>${used.length ? used.map(p => `plan <a href="#/plans/${enc(p)}">${esc(p)}</a>`).join(" · ") : `<span class="mute">No plan yet</span>`}</td></tr>
    </table>
    <div class="actions"><a class="btn primary" href="#/run?suite=${enc(n)}">Run this suite…</a>
      ${editable ? `<a class="btn" href="#/edit/suites/${enc(n)}">Edit suite</a>` : `<a class="btn" href="#/edit/suites/${enc(n)}?copy=1">Copy and edit…</a>`}</div>
    ${cueSource(`suites: ${JSON.stringify(n)}: {\n\tdescription: ${JSON.stringify(s.desc)}\n\ttests: ${JSON.stringify(s.tests)}\n}`)}`;
}

function stepLine(s) {
  if (Array.isArray(s)) return `<a class="mono" href="#/tests/steps/${enc(s[0])}">${esc(s[0])}</a> <span class="mute">Built-in step${s[2] ? " · " + esc(s[2].replace(/\n\s*/g, ", ")) : ""} · times out after ${esc(s[1])}</span>`;
  const k = STEP_KINDS.find(x => x[0] === s.kind);
  return `${esc(s.name || k[1])} <span class="mute">${esc(k[1])}${s.path ? " · " + esc(s.path) : ""} · times out after ${esc(s.timeout || "15m")}</span>`;
}

function testDetail(n) {
  const t = D.tests[n], used = usedByTest(n), editable = (t.origin || "built-in") !== "built-in";
  return `${detailHead(n, t.origin || "built-in")}<p>${esc(t.desc)}</p>
    <table class="kv">
      <tr><th>Steps</th><td>${t.steps.map((s, i) => `<div>${i + 1}. ${stepLine(s)}</div>`).join("")}</td></tr>
      ${t.spare ? `<tr><th>Needs</th><td>${plural(t.spare, "spare machine")}</td></tr>` : ""}
      <tr><th>Labels</th><td>${(t.labels || []).map(l => `<span class="chip mono">${esc(l)}</span>`).join(" ") || `<span class="mute">None</span>`}</td></tr>
      ${t.once ? `<tr><th>Runs</th><td>Once per testbed: later suites that list it reuse its result</td></tr>` : ""}
      <tr><th>Used by</th><td>${used.length ? used.map(s => `suite <a href="#/tests/suites/${enc(s)}">${esc(s)}</a>`).join(" · ") : `<span class="mute">No suite yet</span>`}</td></tr>
    </table>
    <div class="actions">${editable ? `<a class="btn" href="#/edit/tests/${enc(n)}">Edit test</a>` : `<a class="btn" href="#/edit/tests/${enc(n)}?copy=1">Copy and edit…</a>`}</div>`;
}

function stepDetail(n) {
  const [desc, schema] = D.steps[n];
  const used = Object.keys(D.tests).filter(t => D.tests[t].steps.some(s => Array.isArray(s) && s[0] === n));
  return `${detailHead(n, "built-in")}<p>${esc(desc)}</p>
    <table class="kv">
      <tr><th>Parameters</th><td><pre class="log">${esc(schema)}</pre></td></tr>
      <tr><th>Used by</th><td>${used.length ? used.map(t => `test <a href="#/tests/tests/${enc(t)}">${esc(t)}</a>`).join(" · ") : `<span class="mute">No test yet</span>`}</td></tr>
    </table>
    <p class="hint" style="margin-top:12px">To use it, edit a test, add a step, choose <b>Built-in step</b> and pick <span class="mono">${esc(n)}</span>.</p>`;
}

// Edit forms.
function fieldHtml(f, val) {
  const id = "f-" + f.p.replace(/\W/g, "-");
  let ctl;
  if (f.t === "opts") {
    const cur = val === undefined || val === null ? f.d : val;
    ctl = `<span class="seg" role="radiogroup" aria-labelledby="${id}-l">${f.o.map(([v, l]) => `<label><input type="radio" name="${esc(f.p)}" value="${esc(v)}"${cur === v ? " checked" : ""}>${esc(l)}</label>`).join("")}</span>`;
  } else if (f.t === "yaml" || f.t === "lines") {
    ctl = `<textarea id="${id}" class="input mono" rows="${f.t === "yaml" ? 5 : 3}" name="${esc(f.p)}">${esc(val ?? "")}</textarea>`;
  } else if (f.t === "bool") {
    ctl = `<label class="check"><input id="${id}" type="checkbox" name="${esc(f.p)}"${val ? " checked" : ""}> ${esc(f.cl || "")}</label>`;
  } else if (f.t === "int") {
    ctl = `<input id="${id}" class="input num" type="number" min="0" name="${esc(f.p)}" value="${esc(val ?? "")}">`;
  } else {
    ctl = `<input id="${id}" class="input${f.mono ? " mono" : ""}" name="${esc(f.p)}" value="${esc(val ?? "")}">`;
  }
  const hint = [f.h, f.d && !["opts", "bool"].includes(f.t) ? "Leave empty for the default: " + f.d + "." : ""].filter(Boolean).join(" ");
  return `<div class="field"><label class="k" id="${id}-l"${f.t === "opts" ? "" : ` for="${id}"`}>${esc(f.l)}</label><div class="ctl">${ctl}${hint ? `<span class="hint">${esc(hint)}</span>` : ""}</div></div>`;
}

function editFrame(o) {
  const title = o.isNew ? "New " + o.what : o.copy ? "Copy of " + o.name : o.name;
  const del = o.isNew || o.copy ? "" : o.used.length
    ? `<button type="button" class="btn danger" disabled>Delete ${esc(o.what)}</button><span class="disabled-why">Used by ${o.used.map(esc).join(", ")}, so it can't be deleted.</span>`
    : `<button type="button" class="btn danger" data-confirm="Delete ${esc(o.what)} ${esc(o.name)}? It is removed from ./.wendy/wendy-ui.cue." data-toast="Mockup: deleted ${esc(o.name)}">Delete ${esc(o.what)}</button>`;
  return `<div class="crumbs"><a href="${o.back}">${esc(o.backLabel)}</a> › ${esc(title)}</div>
    <form id="editform" onsubmit="return false"><div class="page">
      <h2 class="title">${o.isNew ? "New " + esc(o.what) : o.copy ? "Copy of " + esc(o.what) + " " + esc(o.name) : "Edit " + esc(o.what) + " " + esc(o.name)}</h2>
      <p class="lead" style="margin-top:6px">${esc(o.blurb)}</p>
      ${o.copy ? `<div class="note">${esc(o.name)} is built in. Your copy is saved under a new name in ./.wendy/wendy-ui.cue.</div>` : ""}
      <div class="field"><label class="k" for="f-name">Name</label><div class="ctl"><input id="f-name" class="input" name="name" value="${esc(o.isNew ? "" : o.copy ? o.name + "-copy" : o.name)}"><span class="hint">Letters, digits, - and _.</span></div></div>
      ${o.body}
    </div>
    <div class="savebar">
      <span class="mute" style="font-size:12px;margin-right:auto">Saved to <span class="mono">./.wendy/wendy-ui.cue</span>. Wendy checks it against the schema first.</span>
      ${del}
      <a class="btn" href="${o.back}">Cancel</a>
      <button type="button" class="btn primary" data-act="save" data-back="${o.back}">Save ${esc(o.what)}</button>
    </div></form>`;
}

ACTS.save = t => { toast("Mockup: saved to ./.wendy/wendy-ui.cue"); location.hash = t.dataset.back; };

function pEdit(sub, name, q) {
  const copy = q.get("copy") === "1";
  if (sub === "testbeds") return editTestbed(name, copy);
  if (UNSLUG[sub]) return editPart(UNSLUG[sub], name, copy);
  if (sub === "tests") return editTest(name, copy);
  if (sub === "suites") return editSuite(name, copy);
  return notFound();
}

function editTestbed(name, copy) {
  const t = name ? D.testbeds[name] : { desc: "", parts: {} };
  if (!t) return notFound();
  after.push(() => $("#editform").addEventListener("change", ev => {
    const k = ev.target.dataset.kind;
    if (k) { $("#hint-" + k).textContent = partSummary(k, ev.target.value); $("#open-" + k).href = "#/testbeds/" + SLUG[k] + "/" + enc(ev.target.value); }
  }));
  const pick = k => t.parts[k] || "default";
  const body = `${fieldHtml({ p: "description", l: "Description" }, t.desc)}
    <div class="h4">Parts</div>
    <p class="hint">Pick one of each kind. A part is shared: change it once and every testbed that picks it changes.</p>
    ${PART_KINDS.map(k => `<div class="field"><label class="k" for="p-${k}">${KINDS[k].label}</label><div class="ctl">
      <div style="display:flex;gap:8px;align-items:center"><select id="p-${k}" class="input" data-kind="${k}">${Object.keys(D.parts[k]).map(p => `<option${p === pick(k) ? " selected" : ""}>${esc(p)}</option>`).join("")}</select>
      <a class="btn sm" id="open-${k}" href="#/testbeds/${SLUG[k]}/${enc(pick(k))}">Open</a></div>
      <span class="hint" id="hint-${k}">${esc(partSummary(k, pick(k)))}</span></div></div>`).join("")}
    <div class="h4">Settings</div>
    <p class="hint">More than one part reads these, so the testbed holds them.</p>
    ${Object.entries(SETTINGS).map(([k, s]) => fieldHtml({ p: k, l: s.l, t: "opts", o: s.o, d: s.d, h: s.h }, t[k])).join("")}
    <details class="adv"${t.nat64 || t.mirror ? " open" : ""}><summary>Advanced settings</summary>
      ${fieldHtml({ p: "nat64", l: "NAT64", t: "opts", o: [["", "Default (on with IPv6)"], ["true", "On"], ["false", "Off"]], d: "" }, t.nat64 === undefined ? "" : String(t.nat64))}
      ${fieldHtml({ p: "mirror", l: "Registry mirror", t: "bool", cl: "Pull images through a mirror on this host" }, t.mirror)}
      ${fieldHtml({ p: "upstreams", l: "Mirror upstreams", t: "lines", h: "One registry per line.", d: "ghcr.io, quay.io, registry.k8s.io, gcr.io, docker.io" }, "")}
    </details>
    <div class="h4">Matrix</div>
    ${t.matrix ? matrixHtml(name) + `<p class="hint">Matrices are written in CUE for now.</p>` : `<p class="hint">None: this testbed is one combination. To test several, add a <span class="mono">matrix</span> to it in CUE.</p>`}`;
  return editFrame({ what: "testbed", name, copy: copy || (t.origin === "built-in" && !!name), isNew: !name, used: name ? usedByTestbed(name).map(p => "plan " + p) : [], back: "#/testbeds/testbeds" + (name ? "/" + enc(name) : ""), backLabel: "Testbeds", blurb: BLURBS.testbeds, body });
}

function editPart(kind, name, copy) {
  const p = name ? D.parts[kind][name] : { v: {}, origin: "" };
  if (!p) return notFound();
  const fs = KINDS[kind].fields;
  const one = f => (f.t === "machines" ? machinesTable(p.v) : fieldHtml(f, p.v[f.p]));
  const body = fs.filter(f => !f.adv).map(one).join("") +
    `<details class="adv"${fs.some(f => f.adv && p.v[f.p] !== undefined) ? " open" : ""}><summary>Advanced settings</summary>${fs.filter(f => f.adv).map(one).join("")}</details>`;
  return editFrame({ what: KINDS[kind].one, name, copy: copy || (p.origin === "built-in" && !!name), isNew: !name, used: name ? usedByPart(kind, name).map(t => "testbed " + t) : [], back: "#/testbeds/" + SLUG[kind] + (name ? "/" + enc(name) : ""), backLabel: KINDS[kind].label, blurb: KINDS[kind].blurb, body });
}

function machinesTable(v) {
  return `<div class="field"><span class="k" style="display:block;font-weight:600;margin-bottom:4px">Machines</span>
    <table class="machines"><thead><tr><th></th>${MCOLS.map(c => `<th scope="col">${c[1]}</th>`).join("")}</tr></thead><tbody>
    ${ROLES.map(([r, l]) => `<tr><th scope="row">${l}</th>${MCOLS.map(([c, cl, d]) => `<td><input class="input num" type="number" min="0" aria-label="${l} ${cl.toLowerCase()}" value="${v["machines." + r + "." + c] ?? ""}" placeholder="${d}"></td>`).join("")}</tr>`).join("")}
    </tbody></table>
    <span class="hint">Empty takes the default shown. Spare machines wait unused until a test claims one, e.g. to scale out.</span></div>`;
}

// Test editor: steps are kept in TEST_STEPS while the form is open.
let TEST_STEPS = [];

function editTest(name, copy) {
  const t = name ? D.tests[name] : { desc: "", labels: [], steps: [] };
  if (!t) return notFound();
  TEST_STEPS = t.steps.map(s => (Array.isArray(s) ? { kind: "uses", uses: s[0], timeout: s[1], with: s[2] || "" } : Object.assign({}, s)));
  after.push(() => {
    const box = $("#steps");
    const sync = ev => {
      const el = ev.target;
      if (el.dataset.i === undefined) return;
      TEST_STEPS[+el.dataset.i][el.dataset.key] = el.type === "checkbox" ? el.checked : el.value;
      if (el.dataset.key === "kind" || el.dataset.key === "uses") drawSteps();
    };
    box.addEventListener("input", sync);
    box.addEventListener("change", sync);
    drawSteps();
  });
  const body = `${fieldHtml({ p: "description", l: "Description" }, t.desc)}
    ${fieldHtml({ p: "labels", l: "Labels", h: "Words separated by spaces. Plans and executions can pick tests by label." }, (t.labels || []).join(" "))}
    ${fieldHtml({ p: "spare", l: "Spare machines", t: "int", h: "Machines this test claims from the spare pool, e.g. to scale out.", d: "0" }, t.spare)}
    ${fieldHtml({ p: "once", l: "Run once per testbed", t: "bool", cl: "Later suites that list this test reuse its first result" }, t.once)}
    <div class="h4">Steps</div>
    <div id="steps"></div>
    <button type="button" class="btn" data-act="addStep">＋ Add step</button>`;
  return editFrame({ what: "test", name, copy: copy || ((t.origin || "built-in") === "built-in" && !!name), isNew: !name, used: name ? usedByTest(name).map(s => "suite " + s) : [], back: "#/tests/tests" + (name ? "/" + enc(name) : ""), backLabel: "Tests", blurb: BLURBS.tests, body });
}

function stepField(i, key, label, val, opt = {}) {
  const id = `s${i}-${key}`;
  const ctl = opt.area
    ? `<textarea id="${id}" class="input mono" rows="3" data-i="${i}" data-key="${key}">${esc(val ?? "")}</textarea>`
    : opt.check
      ? `<label class="check"><input id="${id}" type="checkbox" data-i="${i}" data-key="${key}"${val ? " checked" : ""}> ${esc(opt.check)}</label>`
      : opt.options
        ? `<select id="${id}" class="input" data-i="${i}" data-key="${key}">${opt.options.map(o => `<option${o === val ? " selected" : ""}>${esc(o)}</option>`).join("")}</select>`
        : `<input id="${id}" class="input${opt.mono ? " mono" : ""}${opt.num ? " num" : ""}" data-i="${i}" data-key="${key}" value="${esc(val ?? "")}">`;
  return `<div class="field"><label class="k" for="${id}">${esc(label)}</label><div class="ctl">${ctl}${opt.h ? `<span class="hint">${esc(opt.h)}</span>` : ""}</div></div>`;
}

function stepBox(s, i) {
  const k = STEP_KINDS.find(x => x[0] === s.kind);
  const f = (key, label, opt) => stepField(i, key, label, s[key], opt);
  const kindFields = {
    uses: () => f("uses", "Built-in step", { options: Object.keys(D.steps), h: s.uses && D.steps[s.uses] ? D.steps[s.uses][0] : "" }) + f("with", "Parameters", { area: 1, h: "YAML. See this step's parameters under Tests › Built-in steps." }),
    exec: () => f("command", "Command", { area: 1, h: "One argument per line; the first is the program." }) + f("env", "Environment variables", { area: 1, h: "KEY=value, one per line." }),
    container: () => f("image", "Image", { mono: 1 }) + f("entrypoint", "Entrypoint", { area: 1, h: "Optional. One argument per line." }) + f("args", "Arguments", { area: 1, h: "One argument per line." }) + f("env", "Environment variables", { area: 1, h: "KEY=value, one per line." }),
    chainsaw: () => f("path", "Test directory", { mono: 1, h: "Relative to your config directory." }) + f("selector", "Selectors", { area: 1, h: "Optional. One label selector per line." }) + f("image", "Chainsaw image", { mono: 1, h: "Leave empty for the default: ghcr.io/kyverno/chainsaw:v0.2.15." }),
    ginkgo: () => f("binary", "Test binary", { mono: 1, h: "A path to the compiled test binary." }) + f("filter", "Label filter", { mono: 1, h: "Passed to the binary as --ginkgo.label-filter." }) + f("args", "Arguments", { area: 1, h: "One argument per line." }),
  }[s.kind];
  return `<div class="step"><div class="stephead"><b>${i + 1}</b>
      <select class="input" data-i="${i}" data-key="kind" aria-label="What step ${i + 1} runs">${STEP_KINDS.map(([v, l]) => `<option value="${v}"${v === s.kind ? " selected" : ""}>${l}</option>`).join("")}</select>
      <span style="flex:1"></span>
      <button type="button" class="btn sm" data-act="stepMove" data-i="${i}" data-d="-1" aria-label="Move step ${i + 1} up"${i === 0 ? " disabled" : ""}>↑</button>
      <button type="button" class="btn sm" data-act="stepMove" data-i="${i}" data-d="1" aria-label="Move step ${i + 1} down"${i === TEST_STEPS.length - 1 ? " disabled" : ""}>↓</button>
      <button type="button" class="btn sm danger" data-act="stepRemove" data-i="${i}">Remove</button></div>
    <div class="hint">${esc(k[2])}</div>
    ${kindFields()}
    ${f("timeout", "Timeout", { num: 0, h: "e.g. 90s, 15m, 1h. Leave empty for the default: 15m." })}
    <details class="adv"><summary>More</summary>
      ${f("name", "Name", { h: "Shown in the execution tree. Empty shows what the step runs." })}
      ${f("retries", "Retries", { h: "Leave empty for the default: 0." })}
      ${f("when", "Only on combinations with labels", { mono: 1, h: "Skips this step unless the combination's labels match, e.g. ipv6." })}
      ${f("cont", "Continue on failure", { check: "Keep going with the next step if this one fails" })}
    </details></div>`;
}

function drawSteps() {
  $("#steps").innerHTML = TEST_STEPS.map(stepBox).join("") || `<p class="hint">No steps yet.</p>`;
}

ACTS.addStep = () => { TEST_STEPS.push({ kind: "uses", uses: "assert.healthy", timeout: "" }); drawSteps(); };
ACTS.stepMove = t => {
  const i = +t.dataset.i, j = i + +t.dataset.d;
  [TEST_STEPS[i], TEST_STEPS[j]] = [TEST_STEPS[j], TEST_STEPS[i]];
  drawSteps();
};
ACTS.stepRemove = t => { TEST_STEPS.splice(+t.dataset.i, 1); drawSteps(); };

// Suite editor.
let SUITE_TESTS = [];

function editSuite(name, copy) {
  const s = name ? D.suites[name] : { desc: "", tests: [] };
  if (!s) return notFound();
  SUITE_TESTS = s.tests.slice();
  after.push(drawSuite);
  const body = `${fieldHtml({ p: "description", l: "Description" }, s.desc)}
    <div class="h4">Tests, in the order they run</div>
    <div id="suite-tests"></div>
    <div style="display:flex;gap:8px;margin-top:8px"><select id="add-test" class="input" aria-label="Test to add">${Object.keys(D.tests).map(t => `<option>${esc(t)}</option>`).join("")}</select>
      <button type="button" class="btn" data-act="suiteAdd">＋ Add test</button></div>`;
  return editFrame({ what: "suite", name, copy: copy || (s.origin === "built-in" && !!name), isNew: !name, used: name ? usedBySuite(name).map(p => "plan " + p) : [], back: "#/tests/suites" + (name ? "/" + enc(name) : ""), backLabel: "Suites", blurb: BLURBS.suites, body });
}

function drawSuite() {
  $("#suite-tests").innerHTML = SUITE_TESTS.map((t, i) => `<div class="step" style="display:flex;gap:8px;align-items:center">
      <b>${i + 1}</b><span>${esc(t)}</span><span class="hint" style="flex:1">${esc(D.tests[t].desc)}</span>
      <button type="button" class="btn sm" data-act="suiteMove" data-i="${i}" data-d="-1" aria-label="Move ${esc(t)} up"${i === 0 ? " disabled" : ""}>↑</button>
      <button type="button" class="btn sm" data-act="suiteMove" data-i="${i}" data-d="1" aria-label="Move ${esc(t)} down"${i === SUITE_TESTS.length - 1 ? " disabled" : ""}>↓</button>
      <button type="button" class="btn sm danger" data-act="suiteRemove" data-i="${i}">Remove</button></div>`).join("") || `<p class="hint">No tests yet.</p>`;
}

ACTS.suiteAdd = () => { SUITE_TESTS.push($("#add-test").value); drawSuite(); };
ACTS.suiteMove = t => {
  const i = +t.dataset.i, j = i + +t.dataset.d;
  [SUITE_TESTS[i], SUITE_TESTS[j]] = [SUITE_TESTS[j], SUITE_TESTS[i]];
  drawSuite();
};
ACTS.suiteRemove = t => { SUITE_TESTS.splice(+t.dataset.i, 1); drawSuite(); };

// Config files.
function pConfig() {
  const err = D.configError;
  const files = [["~/.config/wendy/wendy.cue", false], ["./.wendy/wendy.cue", false], ["./.wendy/nightly.cue", err], ["./.wendy/wendy-ui.cue", false]];
  return `<div class="page">
    <h2 class="title">Config files</h2>
    <p class="mute">Watching <span class="mono">~/.config/wendy</span> and <span class="mono">./.wendy</span> · last reload 10:58:12
      ${err ? ` · <span class="fail">1 error</span>` : ""} <button class="btn sm" data-toast="Mockup: reloaded">Reload now</button></p>
    <div class="card">${files.map(([f, bad]) => `<div style="margin-top:6px">${bad ? `<span class="fail">✗</span>` : `<span class="pass">✓</span>`} <span class="mono">${f}</span></div>
      ${bad ? `<div class="err-line"><b>./.wendy/nightly.cue:14:24</b>&nbsp; testbeds.versions.matrix.axes.k8s."1.38".workloadCluster: there is no workload cluster part named "1.38". Add one under <span class="mono">parts: workloadCluster:</span>, or pick one of: default, quickstart, 1.36, 1.37.</div>` : ""}`).join("")}
    </div>
    ${err ? `<p>The last valid config is still in use, so everything else keeps working. Fix the file and save; Wendy reloads it.</p>` : `<p class="mute">Wendy reloads these files when they change. The UIs write only to <span class="mono">./.wendy/wendy-ui.cue</span>.</p>`}
  </div>`;
}

// Glossary (mockup only).
function pGlossary() {
  return `<div class="page"><h2 class="title">Glossary</h2>
    <p class="lead" style="margin-top:6px">The words this mockup uses, from ux-redesign/map.md. This page is part of the mockup, not of Wendy.</p>
    <table><tr><th>Term</th><th>Meaning</th><th>Replaces</th></tr>
    ${GLOSSARY.map(([t, m, r]) => `<tr><td><b>${esc(t)}</b></td><td>${esc(m)}</td><td class="mute">${esc(r)}</td></tr>`).join("")}</table>
    <p class="hint" style="margin-top:10px">Statuses: ${Object.keys(ST).map(s => `<span class="${cls(s)}">${glyph(s)} ${word(s)}</span>`).join(" &nbsp; ")}</p></div>`;
}
