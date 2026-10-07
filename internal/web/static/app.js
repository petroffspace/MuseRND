"use strict";

// ---- helpers -----------------------------------------------------------------

const $ = (id) => document.getElementById(id);
const PARTS = ["lead", "chords", "bass", "drums"];
const PART_LABEL = { lead: "Melody", chords: "Chords", bass: "Bass", drums: "Drums" };
// display name of a part (extra parts carry their own label, e.g. "Arpeggio 1")
function partLabel(kind) {
  return PART_LABEL[kind] || currentSong?.parts.find((p) => p.kind === kind)?.label || kind;
}
// colour of a part: CSS variables for the main four, a hue per extra part
function partColor(kind) {
  if (PART_LABEL[kind]) return `var(--${kind})`;
  const i = Number(kind.replace("extra", "")) - 1;
  return `hsl(${(i * 47 + 190) % 360} 62% 52%)`;
}
function partColorValue(kind) { return PART_LABEL[kind] ? css("--" + kind) : partColor(kind); }
const css = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
const fmtTime = (s) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, "0")}`;
const pick = (arr) => arr[Math.floor(Math.random() * arr.length)];

async function api(path, opts) {
  const r = await fetch(path, opts);
  const body = await r.json();
  if (!r.ok) throw new Error(body.error || r.statusText);
  return body;
}

const store = {
  get(k) { try { return JSON.parse(localStorage.getItem("musernd." + k)); } catch { return null; } },
  set(k, v) { try { localStorage.setItem("musernd." + k, JSON.stringify(v)); } catch { /* private mode */ } },
};

// GM program -> FluidR3 soundfont file name (gleitz/midi-js-soundfonts).
const SF_NAMES = ("acoustic_grand_piano bright_acoustic_piano electric_grand_piano honkytonk_piano electric_piano_1 " +
  "electric_piano_2 harpsichord clavinet celesta glockenspiel music_box vibraphone marimba xylophone tubular_bells " +
  "dulcimer drawbar_organ percussive_organ rock_organ church_organ reed_organ accordion harmonica tango_accordion " +
  "acoustic_guitar_nylon acoustic_guitar_steel electric_guitar_jazz electric_guitar_clean electric_guitar_muted " +
  "overdriven_guitar distortion_guitar guitar_harmonics acoustic_bass electric_bass_finger electric_bass_pick " +
  "fretless_bass slap_bass_1 slap_bass_2 synth_bass_1 synth_bass_2 violin viola cello contrabass tremolo_strings " +
  "pizzicato_strings orchestral_harp timpani string_ensemble_1 string_ensemble_2 synth_strings_1 synth_strings_2 " +
  "choir_aahs voice_oohs synth_choir orchestra_hit trumpet trombone tuba muted_trumpet french_horn brass_section " +
  "synth_brass_1 synth_brass_2 soprano_sax alto_sax tenor_sax baritone_sax oboe english_horn bassoon clarinet " +
  "piccolo flute recorder pan_flute blown_bottle shakuhachi whistle ocarina lead_1_square lead_2_sawtooth " +
  "lead_3_calliope lead_4_chiff lead_5_charang lead_6_voice lead_7_fifths lead_8_bass__lead pad_1_new_age " +
  "pad_2_warm pad_3_polysynth pad_4_choir pad_5_bowed pad_6_metallic pad_7_halo pad_8_sweep fx_1_rain " +
  "fx_2_soundtrack fx_3_crystal fx_4_atmosphere fx_5_brightness fx_6_goblins fx_7_echoes fx_8_scifi sitar banjo " +
  "shamisen koto kalimba bagpipe fiddle shanai tinkle_bell agogo steel_drums woodblock taiko_drum melodic_tom " +
  "synth_drum reverse_cymbal guitar_fret_noise breath_noise seashore bird_tweet telephone_ring helicopter " +
  "applause gunshot").split(" ");

const GM_FAMILIES = ["Piano", "Chromatic percussion", "Organ", "Guitar", "Bass", "Strings", "Ensemble", "Brass",
  "Reed", "Pipe", "Synth lead", "Synth pad", "Synth effects", "Ethnic", "Percussive", "Sound effects"];

// ---- state -----------------------------------------------------------------

let OPT = null;          // /api/options
let currentSong = null;  // /api/songs/{id}
let currentJob = null;

// ---- controls --------------------------------------------------------------

function genreOpt() { return OPT.genres.find((g) => g.id === $("genre").value); }
function moodOpt() { return OPT.moods.find((m) => m.id === $("mood").value); }
// tempo the energy slider suggests (same formula as the server)
function defaultBPM() { return Math.round(genreOpt().bpm * (1 + 0.15 * sliderValues().energy)); }

// ---- mood sliders ------------------------------------------------------------
const SLIDERS = [
  ["energy", "Energy", "calm ↔ energetic"],
  ["darkness", "Darkness", "bright ↔ dark"],
  ["density", "Density", "sparse ↔ busy"],
  ["swing", "Swing", "straighter ↔ swung"],
  ["complexity", "Complexity", "simple ↔ complex"],
];
function buildSliders() {
  const box = $("sliders");
  box.innerHTML = "";
  for (const [id, name, hint] of SLIDERS) {
    const row = document.createElement("div");
    row.className = "slider";
    row.innerHTML = `<span>${name}<small>${hint}</small></span><input type="range" id="sl-${id}" min="-100" max="100" step="5" value="0"><span id="sv-${id}">0</span>`;
    box.append(row);
    const input = row.querySelector("input");
    input.oninput = () => {
      $("sv-" + id).textContent = input.value;
      if ($("mood").value !== "custom") $("mood").value = "custom";
      updateTempo();
    };
    input.onchange = saveControls;
  }
}
function sliderValues() {
  const v = {};
  for (const [id] of SLIDERS) v[id] = Number($("sl-" + id)?.value || 0) / 100;
  return v;
}
function setSliders(values) {
  for (const [id] of SLIDERS) {
    const x = Math.round(((values && values[id]) || 0) * 100);
    $("sl-" + id).value = x;
    $("sv-" + id).textContent = x;
  }
}

function fillSelect(sel, items, value) {
  sel.innerHTML = "";
  for (const [v, label] of items) sel.add(new Option(label, v));
  if (value !== undefined && value !== null) sel.value = String(value);
}

function instrumentOptions(part) {
  const sel = document.createElement("select");
  sel.id = "ins-" + part;
  const sug = genreOpt().suggested[part] || [];
  if (part === "drums") {
    const g = document.createElement("optgroup");
    g.label = "Drum kits (GM2/GS)";
    for (const k of OPT.drumKits) g.append(new Option(k.name, k.program));
    sel.append(g);
    return sel;
  }
  const sg = document.createElement("optgroup");
  sg.label = `Suggested for ${$("genre").value}`;
  for (const p of sug) sg.append(new Option(OPT.gm[p], p));
  sel.append(sg);
  GM_FAMILIES.forEach((fam, f) => {
    const g = document.createElement("optgroup");
    g.label = fam;
    for (let p = f * 8; p < f * 8 + 8; p++) g.append(new Option(`${p + 1}. ${OPT.gm[p]}`, p));
    sel.append(g);
  });
  return sel;
}

function buildInstrumentRows(values) {
  const box = $("instruments");
  box.innerHTML = "";
  for (const part of PARTS) {
    const row = document.createElement("div");
    row.className = "insrow";
    row.dataset.part = part;
    const lab = document.createElement("span");
    lab.innerHTML = `<i style="background:var(--${part})"></i>${PART_LABEL[part]}`;
    const sel = instrumentOptions(part);
    // pick the first <option> with the value (suggested group comes first)
    const want = values?.[part] ?? genreOpt().instruments[part].program;
    const opt = [...sel.options].find((o) => o.value === String(want));
    if (opt) opt.selected = true;
    const dice = document.createElement("button");
    dice.className = "ghost";
    dice.textContent = "🎲";
    dice.title = "Random " + PART_LABEL[part].toLowerCase() + " instrument";
    dice.onclick = () => { randomise(part); saveControls(); };
    sel.onchange = saveControls;
    row.append(lab, sel, dice);
    box.append(row);
  }
  updatePartRows();
}

function randomise(part) {
  const sel = $("ins-" + part);
  let choices;
  if (part === "drums") choices = OPT.drumKits.map((k) => k.program);
  else if ($("randMode").value === "any") choices = [...Array(128).keys()];
  else choices = genreOpt().suggested[part];
  const v = String(pick(choices));
  const opt = [...sel.options].find((o) => o.value === v);
  if (opt) opt.selected = true;
}

function selectedInstruments() {
  const out = {};
  for (const p of PARTS) out[p] = Number($("ins-" + p).value);
  return out;
}

// ---- extra parts (beyond melody, chords, bass, drums) ----------------------------

let extras = []; // [{role, program}]
const EXTRA_SUGGEST = {
  arpeggio: [46, 11, 4, 8, 12, 24, 81, 5],
  pad: [88, 89, 90, 91, 94, 95, 48, 50, 52],
  stabs: [61, 62, 0, 4, 56, 57, 16, 5],
  percussion: [39, 54, 56, 60, 61, 62, 63, 64, 69, 70, 75, 76, 81, 45, 47, 50],
};
function suggestFor(role) { return role === "counter" ? genreOpt().suggested.lead : EXTRA_SUGGEST[role]; }
function extraLabel(i) {
  const role = extras[i].role;
  const n = extras.slice(0, i + 1).filter((x) => x.role === role).length;
  return `${OPT.roles.find((r) => r.id === role).name} ${n}`;
}

function extraInstrumentSelect(x) {
  const sel = document.createElement("select");
  const sg = document.createElement("optgroup");
  sg.label = "Suggested";
  if (x.role === "percussion") {
    for (const k of EXTRA_SUGGEST.percussion) sg.append(new Option(OPT.percussion.find((p) => p.program === k).name, k));
    const all = document.createElement("optgroup");
    all.label = "All GM percussion";
    for (const p of OPT.percussion) all.append(new Option(`${p.program}. ${p.name}`, p.program));
    sel.append(sg, all);
  } else {
    for (const p of suggestFor(x.role)) sg.append(new Option(OPT.gm[p], p));
    sel.append(sg);
    GM_FAMILIES.forEach((fam, f) => {
      const g = document.createElement("optgroup");
      g.label = fam;
      for (let p = f * 8; p < f * 8 + 8; p++) g.append(new Option(`${p + 1}. ${OPT.gm[p]}`, p));
      sel.append(g);
    });
  }
  const opt = [...sel.options].find((o) => o.value === String(x.program));
  if (opt) opt.selected = true;
  return sel;
}

function renderExtras() {
  const box = $("extras");
  box.innerHTML = "";
  extras.forEach((x, i) => {
    const row = document.createElement("div");
    row.className = "xrow";
    const lab = document.createElement("span");
    lab.innerHTML = `<i style="background:${partColor("extra" + (i + 1))}"></i>`;
    lab.append(extraLabel(i));
    lab.title = extraLabel(i);
    const role = document.createElement("select");
    for (const r of OPT.roles) role.add(new Option(r.name, r.id));
    role.value = x.role;
    role.title = "What this part plays";
    role.onchange = () => { x.role = role.value; x.program = pick(suggestFor(x.role)); renderExtras(); saveControls(); };
    const sel = extraInstrumentSelect(x);
    sel.onchange = () => { x.program = Number(sel.value); saveControls(); };
    const dice = document.createElement("button");
    dice.className = "ghost";
    dice.textContent = "🎲";
    dice.title = "Random instrument";
    dice.onclick = () => { randomiseExtra(i); renderExtras(); saveControls(); };
    const del = document.createElement("button");
    del.className = "ghost";
    del.textContent = "✕";
    del.title = "Remove this instrument";
    del.onclick = () => { extras.splice(i, 1); renderExtras(); saveControls(); };
    const head = document.createElement("div");
    head.className = "xhead";
    head.append(lab, dice, del);
    const body = document.createElement("div");
    body.className = "xbody";
    body.append(role, sel);
    row.append(head, body);
    box.append(row);
  });
  const total = 4 + extras.length;
  $("partCount").textContent = `${total}/${OPT.maxParts} instruments`;
  $("addExtra").disabled = total >= OPT.maxParts;
}

function randomiseExtra(i) {
  const x = extras[i];
  if (x.role === "percussion") x.program = pick($("randMode").value === "any" ? OPT.percussion.map((p) => p.program) : EXTRA_SUGGEST.percussion);
  else x.program = $("randMode").value === "any" ? Math.floor(Math.random() * 128) : pick(suggestFor(x.role));
}

function selectedParts() {
  return [...document.querySelectorAll("input[name=part]:checked")].map((c) => c.value);
}

function updatePartRows() {
  const on = new Set(["lead", ...selectedParts()]);
  for (const row of document.querySelectorAll(".insrow")) row.classList.toggle("off", !on.has(row.dataset.part));
}

function updateTempo() {
  if ($("bpmAuto").checked) $("bpm").value = defaultBPM();
  const dotted = genreOpt().beatSteps === 6 ? " ♩." : "";
  $("bpmVal").textContent = `${$("bpm").value} bpm${dotted}${$("bpmAuto").checked ? " (auto)" : ""}`;
  updateDuration();
}

function updateDuration() {
  const s = Number($("seconds").value);
  // bar length in seconds from the genre's meter (beats of beatSteps sixteenths)
  const g = genreOpt();
  const barSec = (g.barSteps * 60) / (Number($("bpm").value) * g.beatSteps);
  const bars = Math.max(2, Math.round(s / barSec));
  $("secVal").textContent = `${fmtTime(s)} · ${bars} bars of ${g.meter}`;
}

function settings() {
  return {
    genre: $("genre").value,
    mood: $("mood").value,
    sliders: sliderValues(),
    scale: $("scale").value,
    tonic: Number($("tonic").value),
    seconds: Number($("seconds").value),
    bpm: $("bpmAuto").checked ? 0 : Number($("bpm").value),
    parts: selectedParts(),
    instruments: selectedInstruments(),
    extras: extras.map((x) => ({ role: x.role, program: x.program })),
    quality: $("quality").value,
    useTaste: $("useTaste").checked,
  };
}

function saveControls() { store.set("settings", settings()); }

async function initControls() {
  OPT = await api("/api/options");
  const saved = store.get("settings") || {};
  // genres grouped by family
  const gsel = $("genre");
  gsel.innerHTML = "";
  for (const fam of [...new Set(OPT.genres.map((g) => g.group || "Other"))]) {
    const og = document.createElement("optgroup");
    og.label = fam;
    for (const g of OPT.genres.filter((x) => (x.group || "Other") === fam)) og.append(new Option(g.title || g.id, g.id));
    gsel.append(og);
  }
  gsel.value = OPT.genres.some((g) => g.id === saved.genre) ? saved.genre : "pop";
  fillSelect($("mood"), [...OPT.moods.map((m) => [m.id, m.name]), ["custom", "custom (sliders)"]], saved.mood || "balanced");
  buildSliders();
  setSliders(saved.sliders || moodOpt()?.sliders);
  fillSelect($("scale"), [["auto", "auto (detect)"], ...OPT.scales.map((s) => [s.id, s.name])], saved.scale || "auto");
  fillSelect($("tonic"), [[-1, "auto"], ...OPT.keys.map((k, i) => [i, k])], saved.tonic ?? -1);
  fillSelect($("quality"), OPT.qualities.map((q) => [q.id, q.name]), saved.quality || "normal");
  $("bpm").min = OPT.limits.minBpm;
  $("bpm").max = OPT.limits.maxBpm;
  $("seconds").min = OPT.limits.minSeconds;
  $("seconds").max = OPT.limits.maxSeconds;
  if (saved.seconds) $("seconds").value = saved.seconds;
  if (saved.bpm) { $("bpmAuto").checked = false; $("bpm").value = saved.bpm; }
  if (saved.parts) for (const c of document.querySelectorAll("input[name=part]")) c.checked = saved.parts.includes(c.value);
  $("useTaste").checked = !!saved.useTaste;
  extras = (saved.extras || []).filter((x) => OPT.roles.some((r) => r.id === x.role)).slice(0, OPT.maxParts - 4);
  renderExtras();
  $("addExtra").onclick = () => {
    const role = OPT.roles[extras.length % OPT.roles.length].id;
    extras.push({ role, program: pick(suggestFor(role)) });
    renderExtras();
    saveControls();
  };
  buildInstrumentRows(saved.instruments);
  updateTempo();

  $("genre").onchange = () => { buildInstrumentRows(); renderExtras(); updateTempo(); saveControls(); refreshTaste(); };
  $("useTaste").onchange = saveControls;
  $("vary").onclick = vary;
  $("lockWhole").onclick = () => currentSong && setLockRange(1, currentSong.meta.bars);
  for (const id of ["lockFrom", "lockTo"]) $(id).onchange = () => { const [f, t] = lockRange(); setLockRange(f, t); };
  $("like").onclick = () => rate(1);
  $("dislike").onclick = () => rate(-1);
  $("mood").onchange = () => {
    if ($("mood").value !== "custom") setSliders(moodOpt().sliders);
    updateTempo();
    saveControls();
  };
  for (const id of ["scale", "tonic", "quality", "randMode"]) $(id).onchange = saveControls;
  $("bpm").oninput = () => { $("bpmAuto").checked = false; updateTempo(); };
  $("bpm").onchange = saveControls;
  $("bpmAuto").onchange = () => { updateTempo(); saveControls(); };
  $("seconds").oninput = updateDuration;
  $("seconds").onchange = saveControls;
  for (const c of document.querySelectorAll("input[name=part]")) c.onchange = () => { updatePartRows(); saveControls(); };
  $("randAll").onclick = () => { PARTS.forEach(randomise); extras.forEach((_, i) => randomiseExtra(i)); renderExtras(); saveControls(); };
  $("resetIns").onclick = () => { buildInstrumentRows(); saveControls(); };
  $("generate").onclick = generate;
  $("cancel").onclick = () => currentJob && api(`/api/jobs/${currentJob}/cancel`, { method: "POST" });
}

// ---- generation ------------------------------------------------------------

async function startJob(path, body) {
  saveControls();
  $("generate").disabled = $("vary").disabled = true;
  $("progress").classList.remove("hidden");
  $("empty").classList.add("hidden");
  $("pstatus").textContent = "Starting…";
  $("pbar").style.width = "0";
  drawChart([]);
  drawMeters(null);
  try {
    const { jobId } = await api(path, { method: "POST", body: JSON.stringify(body) });
    currentJob = jobId;
    pollJob(jobId);
  } catch (e) {
    $("pstatus").textContent = "Error: " + e.message;
    $("generate").disabled = $("vary").disabled = false;
  }
}

function generate() { startJob("/api/generate", settings()); }

function vary() {
  if (!currentSong) return;
  const keep = [...document.querySelectorAll("input[name=keep]:checked")].map((c) => c.value);
  const [from, to] = lockRange();
  const whole = from === 1 && to === currentSong.meta.bars;
  startJob(`/api/songs/${currentSong.meta.id}/vary`, {
    keep, from: whole ? 0 : from, to: whole ? 0 : to,
    amount: $("amount").value, quality: $("quality").value, useTaste: $("useTaste").checked,
  });
}

// lockRange returns the locked bars (1-based, inclusive), kept valid.
function lockRange() {
  const bars = currentSong ? currentSong.meta.bars : 1;
  let from = Math.max(1, Math.min(bars, Math.round(Number($("lockFrom").value) || 1)));
  let to = Math.max(1, Math.min(bars, Math.round(Number($("lockTo").value) || bars)));
  if (to < from) [from, to] = [to, from];
  return [from, to];
}

function setLockRange(from, to) {
  $("lockFrom").value = from;
  $("lockTo").value = to;
  roll.draw(player.pos);
  updateTlockHint();
  updateTempoHint();
  updateIlockHint();
}

async function rate(value) {
  if (!currentSong) return;
  const m = currentSong.meta;
  const r = m.rating === value ? 0 : value; // clicking again clears the rating
  const updated = await api(`/api/songs/${m.id}/rating`, { method: "POST", body: JSON.stringify({ rating: r }) });
  m.rating = updated.rating;
  showRating();
  refreshLibrary();
  refreshTaste();
}

function showRating() {
  const r = currentSong ? currentSong.meta.rating : 0;
  $("like").classList.toggle("active", r === 1);
  $("dislike").classList.toggle("active", r === -1);
}

async function refreshTaste() {
  const genre = $("genre").value;
  let t;
  try { t = await api(`/api/taste?genre=${encodeURIComponent(genre)}`); } catch { return; }
  const box = $("tasteInfo");
  if (!t.ready) {
    box.textContent = `Rate at least 2 ${genre} songs with 👍/👎 to teach it your taste (${t.liked}👍 ${t.disliked}👎 so far).`;
    return;
  }
  const more = (t.prefs || []).filter((p) => p.weight > 0).map((p) => p.feature);
  const less = (t.prefs || []).filter((p) => p.weight < 0).map((p) => p.feature);
  let learned = "";
  if (more.length) learned += ` You like more <b>${more.join(", ")}</b>`;
  if (less.length) learned += `${more.length ? "; less" : " You like less"} <b>${less.join(", ")}</b>`;
  if (!t.disliked) learned = " Steering towards songs like the ones you liked";
  if (!t.liked) learned = " Steering away from songs like the ones you disliked";
  const grow = t.weight < t.maxWeight ? `; grows to ${Math.round(t.maxWeight * 100)}% with 6 ratings` : "";
  box.innerHTML = `Learned from ${t.liked}👍 ${t.disliked}👎 in ${genre} (${Math.round(t.weight * 100)}% of the score${grow}).${learned ? learned + "." : ""}`;
}

async function pollJob(id) {
  let j;
  try { j = await api(`/api/jobs/${id}`); } catch (e) { $("pstatus").textContent = "Error: " + e.message; $("generate").disabled = false; return; }
  const best = j.best ? ` · best ${j.best.total.toFixed(1)} · ${j.best.key}` : "";
  const status = { queued: "Waiting for the previous search…", running: j.parent ? "Evolving a variation" : "Evolving", done: "Done", cancelled: "Cancelled", failed: "Failed" }[j.status];
  const taste = j.taste && j.taste.weight ? " · using your taste" : "";
  $("pstatus").textContent = `${status} · generation ${j.generations.toLocaleString()}${best}${taste}${j.error ? " · " + j.error : ""}`;
  $("pbar").style.width = `${Math.min(100, (100 * j.elapsed) / j.budget)}%`;
  $("cancel").classList.toggle("hidden", !(j.status === "queued" || j.status === "running"));
  drawChart(j.history, j.budget);
  drawMeters(j.best);
  if (j.status === "queued" || j.status === "running") {
    setTimeout(() => pollJob(id), 400);
    return;
  }
  currentJob = null;
  try {
    if (j.status === "done") {
      $("pbar").style.width = "100%";
      await refreshLibrary();
      await loadSong(j.songId);
    }
  } finally {
    $("generate").disabled = $("vary").disabled = false;
  }
}

function drawChart(history, budget = 45) {
  const c = $("chart");
  const w = (c.width = c.clientWidth * devicePixelRatio);
  const h = (c.height = 120 * devicePixelRatio);
  const g = c.getContext("2d");
  g.clearRect(0, 0, w, h);
  const lo = 40, hi = 100;
  const y = (v) => h - ((Math.max(lo, v) - lo) / (hi - lo)) * (h - 10) - 5;
  const x = (t) => (t / budget) * w;
  g.strokeStyle = css("--grid");
  g.fillStyle = css("--muted");
  g.font = `${11 * devicePixelRatio}px system-ui`;
  for (const v of [50, 70, 90]) {
    g.beginPath(); g.moveTo(0, y(v)); g.lineTo(w, y(v)); g.stroke();
    g.fillText(v, 4, y(v) - 3);
  }
  if (!history.length) return;
  g.strokeStyle = css("--accent");
  g.lineWidth = 2 * devicePixelRatio;
  g.beginPath();
  history.forEach((p, i) => (i ? g.lineTo(x(p.t), y(p.total)) : g.moveTo(x(p.t), y(p.total))));
  g.stroke();
}

function drawMeters(best) {
  const box = $("pparts");
  box.innerHTML = "";
  if (!best) return;
  const rows = [["melody", best.melody, "lead"], ["harmony", best.harmony, "chords"], ["bass", best.bass, "bass"], ["drums", best.drums, "drums"], ["extras", best.extras || 0, "extras"]];
  for (const [name, v, color] of rows) {
    if (name !== "melody" && v === 0) continue;
    const r = document.createElement("div");
    r.className = "meter";
    const fill = color === "extras" ? partColor("extra1") : `var(--${color})`;
    r.innerHTML = `<span>${name}</span><div><b style="width:${v}%;background:${fill}"></b></div><span>${v.toFixed(0)}</span>`;
    box.append(r);
  }
}

// ---- song view ---------------------------------------------------------------

async function loadSong(id) {
  player.stop();
  currentSong = await api(`/api/songs/${id}`);
  const m = currentSong.meta;
  $("song").classList.remove("hidden");
  $("empty").classList.add("hidden");
  $("stitle").textContent = m.title;
  // a part may switch instrument mid-song (instrument lock): list its sounds in order
  const byKind = new Map();
  for (const p of currentSong.parts) byKind.set(p.kind, [...(byKind.get(p.kind) || []), p.name]);
  const parts = [...byKind].map(([k, names]) => `${partLabel(k)}: ${[...new Set(names)].join(" → ")}`).join(" · ");
  const keyChange = m.keyEnd ? ` · key change to ${m.keyEnd}` : "";
  const ts = m.settings && m.settings.tempoSection;
  const tempo = ts ? `${m.bpm} bpm (bars ${ts.from}–${ts.to}: ${ts.bpm})` : `${m.bpm} bpm`;
  const meter = m.meter && m.meter !== "4/4" ? ` · ${m.meter}` : "";
  $("smeta").textContent = `${fmtTime(m.seconds)} · ${m.bars} bars${meter} · ${tempo}${keyChange} · ${parts}`;
  const s = m.score;
  const chips = [["total", s.total], ["melody", s.melody], ["harmony", s.harmony], ["bass", s.bass], ["drums", s.drums], ["extras", s.extras || 0]];
  if (m.taste && m.taste.weight) chips.push(["taste", m.tasteScore || 0]);
  $("sscore").innerHTML = chips
    .filter(([k, v]) => k === "total" || k === "melody" || k === "taste" || v > 0)
    .map(([k, v]) => `<span class="chip">${k} <b>${v.toFixed(0)}</b></span>`).join("");
  $("chords").textContent = currentSong.chords.map((c) => c.name).join(" │ ");
  const lin = $("slineage");
  lin.classList.toggle("hidden", !m.parent);
  if (m.parent) {
    lin.innerHTML = "";
    const a = document.createElement("a");
    a.href = "#song=" + m.parent;
    a.textContent = m.parentTitle || "original";
    const where = m.lockFrom ? ` in bars ${m.lockFrom}–${m.lockTo}` : "";
    lin.append("↳ variation of ", a, ` · locked ${m.kept && m.kept.length ? m.kept.join(", ") + where : "nothing"} · ${m.amount}`);
  }
  showRating();
  const present = new Set(["melody", ...currentSong.parts.map((p) => p.kind)]);
  // lock checkboxes for the song's extra parts (percussion cannot be
  // transposed or switch instrument)
  const xparts = [];
  for (const p of currentSong.parts) if (p.kind.startsWith("extra") && !xparts.some((x) => x.kind === p.kind)) xparts.push(p);
  for (const [box, name, pitchedOnly] of [["keepX", "keep", false], ["tlockX", "tlock", true], ["ilockX", "ilock", true]]) {
    $(box).innerHTML = "";
    for (const p of xparts) {
      if (pitchedOnly && p.drums) continue;
      const l = document.createElement("label");
      l.className = "check";
      l.innerHTML = `<input type="checkbox" name="${name}" value="${p.kind}"> `;
      l.append(p.label.toLowerCase());
      $(box).append(l);
    }
  }
  for (const c of document.querySelectorAll("input[name=tlock]")) c.onchange = updateTlockHint;
  for (const c of document.querySelectorAll("input[name=ilock]")) c.onchange = updateIlockHint;
  // edit row: transpose choices show the key they lead to
  const base = m.baseTonic ?? 0;
  fillSelect($("transpose"), Array.from({ length: 25 }, (_, i) => {
    const t = i - 12;
    const sign = t > 0 ? "+" : t < 0 ? "−" : "±";
    return [t, `${sign}${Math.abs(t)} · ${OPT.keys[(((base + t) % 12) + 12) % 12]} ${m.scaleName || ""}`];
  }), m.transpose || 0);
  const tl = (m.settings && m.settings.transposeLock) || { parts: [] };
  for (const c of document.querySelectorAll("input[name=tlock]")) {
    c.checked = (tl.parts || []).includes(c.value);
    c.disabled = !present.has(c.value === "melody" ? "lead" : c.value);
    c.parentElement.classList.toggle("na", c.disabled);
  }
  $("songBpm").value = m.bpm;
  $("songBpm").min = OPT.limits.minBpm;
  $("songBpm").max = OPT.limits.maxBpm;
  $("resetEdit").disabled = !m.transpose && (!m.origBpm || m.origBpm === m.bpm);
  for (const id of ["lockFrom", "lockTo"]) $(id).max = m.bars;
  const tsec = (m.settings && m.settings.tempoSection) || null;
  $("lockFrom").value = tl.from || (tsec && tsec.from) || 1;
  $("lockTo").value = tl.to || (tsec && tsec.to) || m.bars;
  $("tempoLock").checked = !!tsec;
  const ilock = (m.settings && m.settings.instrumentLock) || null;
  if (ilock && !tl.from && !tsec) { $("lockFrom").value = ilock.from; $("lockTo").value = ilock.to; }
  for (const c of document.querySelectorAll("input[name=ilock]")) {
    c.checked = !!ilock && ilock.parts.includes(c.value);
    c.disabled = !present.has(c.value === "melody" ? "lead" : c.value);
    c.parentElement.classList.toggle("na", c.disabled);
  }
  updateTlockHint();
  updateTempoHint();
  updateIlockHint();
  for (const c of document.querySelectorAll("input[name=keep]")) {
    c.disabled = !present.has(c.value);
    c.parentElement.classList.toggle("na", c.disabled);
  }
  for (const [el, f] of [["dlMid", "mid"], ["dlXml", "musicxml"], ["dlGen", "genome"]]) $(el).href = `/api/songs/${id}/download/${f}`;
  $("mutes").innerHTML = "";
  for (const kind of new Set(currentSong.parts.map((p) => p.kind))) {
    const b = document.createElement("button");
    b.className = "ghost";
    b.style.borderColor = partColor(kind);
    b.textContent = partLabel(kind);
    b.title = "Mute / unmute";
    b.onclick = () => { b.classList.toggle("muted-part"); player.setMute(kind, b.classList.contains("muted-part")); };
    $("mutes").append(b);
  }
  for (const li of document.querySelectorAll("#library li")) li.classList.toggle("current", li.dataset.id === id);
  history.replaceState(null, "", "#song=" + id);
  roll.render(currentSong);
  updateTime(0);
  player.prepare(currentSong); // start fetching sounds early
}

function updateTime(pos) {
  const d = currentSong ? currentSong.duration : 0;
  $("time").textContent = `${fmtTime(pos)} / ${fmtTime(d)}`;
}

// ---- piano roll -------------------------------------------------------------

const roll = {
  song: null, pps: 40, layer: null,
  render(song) {
    this.song = song;
    const wrap = $("rollwrap");
    const c = $("roll");
    const H = 300;
    this.pps = Math.max(wrap.clientWidth / song.duration, 24);
    const W = Math.ceil(song.duration * this.pps);
    c.style.width = W + "px";
    c.width = W * devicePixelRatio;
    c.height = H * devicePixelRatio;
    const layer = document.createElement("canvas");
    layer.width = c.width; layer.height = c.height;
    const g = layer.getContext("2d");
    g.scale(devicePixelRatio, devicePixelRatio);

    const melodic = song.parts.filter((p) => !p.drums).flatMap((p) => p.notes.map((n) => n.p));
    const lo = Math.min(...melodic) - 2, hi = Math.max(...melodic) + 2;
    const drumLane = song.parts.some((p) => p.drums) ? 46 : 0;
    const top = 18, pitchH = H - top - drumLane - 4;
    const y = (p) => top + ((hi - p) / (hi - lo)) * pitchH;
    const rowH = Math.max(2, pitchH / (hi - lo));

    g.font = "11px system-ui";
    g.strokeStyle = css("--grid");
    g.fillStyle = css("--muted");
    for (const t of song.barTimes.slice(0, -1)) {
      const x = t * this.pps;
      g.beginPath(); g.moveTo(x, 0); g.lineTo(x, H); g.stroke();
    }
    song.chords.forEach((ch, i) => {
      const w = (song.barTimes[i + 1] - song.barTimes[i]) * this.pps;
      if (w > 26) g.fillText(ch.name, ch.t * this.pps + 3, 12);
    });
    const extraKinds = [...new Set(song.parts.filter((p) => !p.drums && p.kind.startsWith("extra")).map((p) => p.kind))];
    const order = ["chords", "bass", ...extraKinds, "lead"];
    for (const kind of order) {
      g.fillStyle = partColorValue(kind);
      g.globalAlpha = kind === "chords" ? 0.45 : kind.startsWith("extra") ? 0.7 : 0.9;
      for (const part of song.parts.filter((p) => p.kind === kind)) {
        for (const n of part.notes) g.fillRect(n.t * this.pps, y(n.p) - rowH / 2, Math.max(2, n.d * this.pps - 1), rowH);
      }
    }
    g.globalAlpha = 1;
    const drumParts = song.parts.filter((p) => p.drums);
    const drums = drumParts.length ? { notes: drumParts.flatMap((p) => p.notes) } : null;
    if (drums) {
      const keys = [...new Set(drums.notes.map((n) => n.p))].sort((a, b) => a - b);
      const lane = (k) => H - drumLane + 4 + (keys.indexOf(k) / Math.max(1, keys.length)) * (drumLane - 8);
      g.fillStyle = css("--drums");
      for (const n of drums.notes) g.fillRect(n.t * this.pps, lane(n.p), 2, Math.max(2, (drumLane - 8) / keys.length - 1));
    }
    this.layer = layer;
    this.draw(0);
    // click seeks; dragging selects the bars to lock in a variation
    const barAt = (x) => {
      const t = x / this.pps, bt = song.barTimes;
      let b = 1;
      while (b < song.meta.bars && bt[b] <= t) b++;
      return b;
    };
    c.onmousedown = (e) => {
      const r = c.getBoundingClientRect();
      const x0 = e.clientX - r.left;
      let dragging = false;
      const move = (ev) => {
        const x = ev.clientX - r.left;
        if (!dragging && Math.abs(x - x0) < 6) return;
        dragging = true;
        const a = barAt(Math.min(x0, x)), b = barAt(Math.max(x0, x));
        setLockRange(a, b);
      };
      const up = (ev) => {
        window.removeEventListener("mousemove", move);
        window.removeEventListener("mouseup", up);
        if (!dragging) player.seek((ev.clientX - r.left) / this.pps);
      };
      window.addEventListener("mousemove", move);
      window.addEventListener("mouseup", up);
    };
  },
  draw(pos) {
    const c = $("roll");
    if (!this.layer) return;
    const g = c.getContext("2d");
    g.clearRect(0, 0, c.width, c.height);
    g.drawImage(this.layer, 0, 0);
    // locked section for variations (not shown when it is the whole song)
    if (this.song) {
      const [from, to] = lockRange();
      const bars = this.song.meta.bars;
      if (!(from === 1 && to === bars)) {
        const k = this.pps * devicePixelRatio, bt = this.song.barTimes;
        const x0 = bt[from - 1] * k, w = (bt[to] - bt[from - 1]) * k;
        g.fillStyle = css("--accent");
        g.globalAlpha = 0.12;
        g.fillRect(x0, 0, w, c.height);
        g.globalAlpha = 0.9;
        g.fillRect(x0, 0, w, 3 * devicePixelRatio);
        g.globalAlpha = 1;
      }
    }
    const x = pos * this.pps * devicePixelRatio;
    g.fillStyle = css("--playhead");
    g.fillRect(x, 0, 2 * devicePixelRatio, c.height);
    const wrap = $("rollwrap");
    const vx = pos * this.pps;
    if (player.playing && (vx < wrap.scrollLeft || vx > wrap.scrollLeft + wrap.clientWidth - 40)) wrap.scrollLeft = vx - 40;
  },
};

// ---- player -------------------------------------------------------------------

// Drums are synthesized (no GM percussion soundfont is published); melodic
// parts use FluidR3_GM samples, with a simple synth fallback when offline.
// MIDI channel of each part (as in the exported MIDI)
const PART_CHANNEL = { lead: 0, chords: 1, bass: 2, drums: 9 };

const player = {
  ctx: null, master: null, gains: {}, instruments: {}, song: null, loadedFor: null,
  playing: false, pos: 0, startedAt: 0, timer: null, idx: {}, live: [], loading: null,
  // "" plays the default online FluidR3 sounds; otherwise the name of an
  // uploaded SoundFont, played by SpessaSynth from the song's own MIDI file
  source: "", sp: null,

  async setSource(name) {
    this.stop();
    this.source = name;
  },

  async spessa() {
    const ctx = this.ensureContext();
    if (!this.sp) {
      const lib = await import("./vendor/spessasynth.js");
      await ctx.audioWorklet.addModule("./vendor/spessasynth_processor.min.js");
      const synth = new lib.WorkletSynthesizer(ctx);
      synth.connect(this.master);
      await synth.isReady;
      const seq = new lib.Sequencer(synth);
      seq.loopCount = 0;
      this.sp = { synth, seq, bank: null, song: null };
    }
    const sp = this.sp;
    if (sp.bank !== this.source) {
      $("loading").textContent = "loading SoundFont…";
      const r = await fetch(`/api/soundfonts/${encodeURIComponent(this.source)}`);
      if (!r.ok) throw new Error(`SoundFont ${this.source} is not available`);
      await sp.synth.soundBankManager.addSoundBank(await r.arrayBuffer(), "main");
      sp.bank = this.source;
    }
    if (sp.song !== this.song) {
      $("loading").textContent = "loading song…";
      const mid = await (await fetch(`/api/songs/${this.song.meta.id}/download/mid`)).arrayBuffer();
      sp.seq.loadNewSongList([{ binary: mid, fileName: "song.mid" }]);
      sp.seq.pause();
      sp.song = this.song;
    }
    return sp;
  },

  applyMutes() {
    if (!this.sp || !this.song) return;
    // a channel is silent when every part on it is muted (percussion extras
    // share the drum channel)
    const byChannel = {};
    for (const p of this.song.parts) (byChannel[p.channel] ||= new Set()).add(p.kind);
    for (const [ch, kinds] of Object.entries(byChannel)) {
      const muted = [...kinds].every((k) => (this.muted || {})[k]);
      this.sp.synth.midiChannels[ch]?.setSystemParameter("isMuted", muted);
    }
  },

  async playCustom() {
    const ctx = this.ensureContext();
    await ctx.resume();
    $("loading").classList.remove("hidden");
    let sp;
    try {
      sp = await this.spessa();
    } catch (e) {
      alert("SoundFont playback failed: " + e.message);
      return;
    } finally {
      $("loading").classList.add("hidden");
      $("loading").textContent = "loading sounds…";
    }
    if (this.pos >= this.song.duration) this.pos = 0;
    this.applyMutes();
    sp.seq.currentTime = this.pos;
    sp.seq.play();
    this.playing = true;
    $("play").textContent = "❚❚";
    this.frame();
  },

  ensureContext() {
    if (!this.ctx) {
      this.ctx = new AudioContext();
      this.master = this.ctx.createGain();
      this.master.gain.value = Number($("volume").value);
      this.master.connect(this.ctx.destination);
      const len = this.ctx.sampleRate;
      this.noise = this.ctx.createBuffer(1, len, this.ctx.sampleRate);
      const d = this.noise.getChannelData(0);
      for (let i = 0; i < len; i++) d[i] = Math.random() * 2 - 1;
    }
    return this.ctx;
  },

  prepare(song) {
    this.song = song;
    if (this.loadedFor === song) return this.loading;
    this.loadedFor = song;
    if (!this.ctx) return null; // sounds load on first play (needs a user gesture)
    return this.loadSounds(song);
  },

  loadSounds(song) {
    const ctx = this.ensureContext();
    this.instruments = {};
    this.muted = {};
    this.cache = this.cache || {}; // loaded sounds, by part kind + program, for the whole session
    const jobs = [];
    // one volume (mute) control per part kind, kept for the session; one
    // sound per play part, since a part can switch instrument mid-song
    song.parts.forEach((p, i) => {
      if (!this.gains[p.kind]) {
        this.gains[p.kind] = ctx.createGain();
        this.gains[p.kind].connect(this.master);
      }
      this.gains[p.kind].gain.value = 1;
      if (p.drums) return;
      const key = `${p.kind}:${p.program}`;
      if (!this.cache[key]) {
        this.cache[key] = Soundfont.instrument(ctx, SF_NAMES[p.program], { soundfont: "FluidR3_GM", destination: this.gains[p.kind] })
          .catch(() => { delete this.cache[key]; return null; }); // fallback synth; retry next time
      }
      jobs.push(this.cache[key].then((inst) => (this.instruments[i] = inst)));
    });
    $("loading").classList.remove("hidden");
    this.loading = Promise.all(jobs).finally(() => $("loading").classList.add("hidden"));
    return this.loading;
  },

  setMute(kind, muted) {
    if (this.gains[kind]) this.gains[kind].gain.value = muted ? 0 : 1;
    this.muted = { ...(this.muted || {}), [kind]: muted };
    this.applyMutes();
  },

  async toggle() { this.playing ? this.pause() : await this.play(); },

  async play() {
    if (!this.song) return;
    if (this.source) return this.playCustom();
    const ctx = this.ensureContext();
    await ctx.resume();
    if (this.loadedFor !== this.song || !this.loading) { this.loadedFor = this.song; this.loadSounds(this.song); }
    await this.loading;
    for (const [k, m] of Object.entries(this.muted || {})) if (this.gains[k]) this.gains[k].gain.value = m ? 0 : 1;
    if (this.pos >= this.song.duration) this.pos = 0;
    this.playing = true;
    $("play").textContent = "❚❚";
    this.startedAt = ctx.currentTime - this.pos + 0.05;
    this.song.parts.forEach((p, k) => {
      let i = 0;
      while (i < p.notes.length && p.notes[i].t < this.pos) i++;
      this.idx[k] = i;
    });
    this.schedule();
    this.timer = setInterval(() => this.schedule(), 50);
    this.frame();
  },

  schedule() {
    const ctx = this.ctx, horizon = ctx.currentTime + 0.4;
    this.song.parts.forEach((p, k) => {
      let i = this.idx[k];
      while (i < p.notes.length && this.startedAt + p.notes[i].t < horizon) {
        const n = p.notes[i++];
        const when = Math.max(ctx.currentTime, this.startedAt + n.t);
        if (p.drums) this.drum(n.p, n.v / 127, when, this.gains[p.kind]);
        else this.note(k, p.kind, n, when);
      }
      this.idx[k] = i;
    });
    if (ctx.currentTime - this.startedAt > this.song.duration + 1.5) this.stop();
  },

  note(k, kind, n, when) {
    const inst = this.instruments[k];
    const gain = (n.v / 127) * (kind === "chords" ? 0.7 : 1);
    if (inst) { inst.play(n.p, when, { duration: n.d, gain }); return; }
    const ctx = this.ctx, o = ctx.createOscillator(), g = ctx.createGain();
    o.type = kind === "bass" ? "sawtooth" : "triangle";
    o.frequency.value = 440 * 2 ** ((n.p - 69) / 12);
    g.gain.setValueAtTime(0, when);
    g.gain.linearRampToValueAtTime(gain * 0.25, when + 0.01);
    g.gain.setTargetAtTime(0, when + n.d, 0.08);
    o.connect(g).connect(this.gains[kind]);
    o.start(when); o.stop(when + n.d + 0.5);
    this.live.push(o);
  },

  // Synthesized GM percussion.
  drum(key, vel, when, out) {
    const ctx = this.ctx;
    const env = (node, peak, decay, dur = decay * 4) => {
      const g = ctx.createGain();
      g.gain.setValueAtTime(peak * vel, when);
      g.gain.exponentialRampToValueAtTime(0.001, when + dur);
      node.connect(g).connect(out);
      node.start(when); node.stop(when + dur + 0.05);
      this.live.push(node);
      return node;
    };
    const noise = (type, freq, q = 1) => {
      const s = ctx.createBufferSource();
      s.buffer = this.noise;
      const f = ctx.createBiquadFilter();
      f.type = type; f.frequency.value = freq; f.Q.value = q;
      s.connect(f);
      return { start: (t) => s.start(t, Math.random() * 0.5), stop: (t) => s.stop(t), connect: (n) => f.connect(n), _s: s };
    };
    const tone = (f0, f1, sweep) => {
      const o = ctx.createOscillator();
      o.frequency.setValueAtTime(f0, when);
      if (f1) o.frequency.exponentialRampToValueAtTime(f1, when + sweep);
      return o;
    };
    switch (key) {
      case 35: case 36: env(tone(150, 45, 0.12), 1.0, 0.09, 0.45); break;           // kick
      case 37: env(noise("bandpass", 2200, 3), 0.6, 0.01, 0.05); env(tone(820), 0.25, 0.01, 0.04); break; // side stick
      case 38: case 40: env(noise("highpass", 1200), 0.55, 0.05, 0.22); env(tone(185), 0.35, 0.03, 0.12); break; // snare
      case 39: for (const d of [0, 0.012, 0.024]) { const n = noise("bandpass", 1300, 1.2); const g = ctx.createGain();
        g.gain.setValueAtTime(0.5 * vel, when + d); g.gain.exponentialRampToValueAtTime(0.001, when + d + 0.15);
        n.connect(g); g.connect(out); n.start(when + d); n.stop(when + d + 0.2); this.live.push(n._s); } break; // clap
      case 42: case 44: env(noise("highpass", 7500), 0.35, 0.012, 0.06); break;    // closed hat
      case 46: env(noise("highpass", 7000), 0.3, 0.08, 0.35); break;                // open hat
      case 49: case 57: case 52: case 55: env(noise("highpass", 4500), 0.35, 0.3, 1.6); break; // crash
      case 51: case 59: case 53: env(noise("bandpass", 6000, 0.7), 0.22, 0.2, 0.9); env(tone(3200), 0.03, 0.2, 0.6); break; // ride
      case 70: case 82: case 69: env(noise("bandpass", 5500, 1.5), 0.3, 0.02, 0.09); break; // shaker, maracas, cabasa
      case 41: case 43: case 45: case 47: case 48: case 50: { // toms, low to high
        const f = { 41: 80, 43: 95, 45: 110, 47: 130, 48: 150, 50: 175 }[key];
        env(tone(f * 1.6, f, 0.08), 0.8, 0.08, 0.4);
        break;
      }
      case 60: case 61: env(tone(key === 60 ? 420 : 300, key === 60 ? 380 : 260, 0.03), 0.6, 0.03, 0.15); break; // bongos
      case 62: case 63: case 64: { // congas: mute, open, low
        const f = { 62: 330, 63: 280, 64: 200 }[key];
        env(tone(f * 1.15, f, 0.03), 0.65, key === 62 ? 0.02 : 0.06, key === 62 ? 0.1 : 0.3);
        break;
      }
      case 65: case 66: env(tone(key === 65 ? 520 : 380), 0.4, 0.05, 0.25); env(noise("bandpass", 3000, 1), 0.15, 0.02, 0.1); break; // timbales
      case 56: { // cowbell: two detuned square tones
        for (const f of [540, 800]) { const o = tone(f); o.type = "square"; env(o, 0.12, 0.05, 0.3); }
        break;
      }
      case 54: env(noise("highpass", 6500), 0.35, 0.06, 0.3); env(tone(5200), 0.03, 0.05, 0.25); break; // tambourine
      case 75: case 76: case 77: env(tone({ 75: 2500, 76: 1200, 77: 900 }[key]), 0.5, 0.012, 0.06); break; // claves, wood blocks
      case 80: case 81: env(tone(4200), 0.18, key === 80 ? 0.05 : 0.5, key === 80 ? 0.2 : 1.5); break; // triangle
      case 67: case 68: env(tone(key === 67 ? 900 : 650), 0.3, 0.05, 0.25); break; // agogo
      default: env(noise("bandpass", 1000 + key * 40, 1), 0.3, 0.04, 0.15);
    }
  },

  frame() {
    if (!this.playing) return;
    const now = this.source && this.sp ? this.sp.seq.currentTime : this.ctx.currentTime - this.startedAt;
    if (this.source && (this.sp.seq.isFinished || now >= this.song.duration - 0.02)) { this.stop(); return; }
    const pos = Math.min(this.song.duration, now);
    this.pos = Math.max(0, pos);
    roll.draw(this.pos);
    updateTime(this.pos);
    requestAnimationFrame(() => this.frame());
  },

  silence() {
    if (this.sp) {
      this.sp.seq.pause();
      this.sp.synth.stopAll(true);
    }
    clearInterval(this.timer);
    for (const inst of Object.values(this.instruments)) inst && inst.stop();
    for (const n of this.live) { try { n.stop(); } catch { /* already stopped */ } }
    this.live = [];
  },

  pause() {
    if (!this.playing) return;
    this.playing = false;
    this.silence();
    $("play").textContent = "▶";
  },

  stop() {
    this.pause();
    this.silence();
    this.pos = 0;
    roll.draw(0);
    if (currentSong) updateTime(0);
  },

  seek(t) {
    const was = this.playing;
    this.pause();
    this.pos = Math.max(0, Math.min(t, this.song ? this.song.duration : 0));
    roll.draw(this.pos);
    updateTime(this.pos);
    if (was) this.play();
  },
};

$("play").onclick = () => player.toggle();
$("stop").onclick = () => player.stop();
$("volume").oninput = () => player.master && (player.master.gain.value = Number($("volume").value));
document.addEventListener("keydown", (e) => {
  if (e.code === "Space" && !["INPUT", "SELECT", "BUTTON", "TEXTAREA"].includes(document.activeElement.tagName) && currentSong) {
    e.preventDefault();
    player.toggle();
  }
});
async function editSong(change) {
  if (!currentSong) return;
  const id = currentSong.meta.id;
  const was = player.playing, pos = player.pos, oldDur = currentSong.duration;
  player.stop();
  try {
    await api(`/api/songs/${id}/edit`, { method: "POST", body: JSON.stringify(change) });
    player.loadedFor = null;
    await refreshLibrary();
    await loadSong(id);
    // keep the place in the song (scaled if the tempo changed)
    player.seek((pos * currentSong.duration) / oldDur);
    if (was) player.play();
  } catch (e) {
    alert(e.message);
    loadSong(id);
  }
}
// Transposing applies with the button: amount, locked parts and the
// selected bars together.
$("applyTranspose").onclick = () => {
  const parts = [...document.querySelectorAll("input[name=tlock]:checked")].map((c) => c.value);
  const [from, to] = lockRange();
  const whole = from === 1 && to === currentSong.meta.bars;
  const lock = parts.length ? { parts, from: whole ? 0 : from, to: whole ? 0 : to } : null;
  editSong({ transpose: Number($("transpose").value), transposeLock: lock });
};
function updateTlockHint() {
  const t = Number($("transpose").value);
  const locked = [...document.querySelectorAll("input[name=tlock]:checked")].length;
  const pitched = [...document.querySelectorAll("input[name=tlock]:not(:disabled)")].length;
  const [from, to] = lockRange();
  const whole = currentSong && from === 1 && to === currentSong.meta.bars;
  const h = $("tlockHint");
  let msg = "";
  if (locked && locked < pitched && t % 12 !== 0) msg = "Locked parts stay in the old key and will clash with the transposed ones — fine for effects, or transpose by ±12 for an octave shift.";
  else if (locked === pitched && whole && t !== 0) msg = "Everything is locked in the whole song, so nothing moves — select some bars for a key change.";
  else if (locked === pitched && !whole && t !== 0) msg = `Key change: ${barsText(from, to).toLowerCase()} stay${from === to ? "s" : ""}, the rest moves ${t > 0 ? "up" : "down"} ${Math.abs(t)}.`;
  h.textContent = msg;
  h.classList.toggle("hidden", !msg);
}
$("transpose").onchange = updateTlockHint;
for (const c of document.querySelectorAll("input[name=tlock]")) c.onchange = updateTlockHint;
// Tempo applies with its button: the new tempo, and optionally the selected
// bars keeping the tempo they have now.
$("applyTempo").onclick = () => {
  const v = Math.round(Number($("songBpm").value));
  if (!(v >= OPT.limits.minBpm && v <= OPT.limits.maxBpm)) { $("songBpm").value = currentSong.meta.bpm; return; }
  const [from, to] = lockRange();
  editSong({ bpm: v, keepTempo: $("tempoLock").checked ? { from, to } : null });
};
function barsText(from, to) { return from === to ? `Bar ${from}` : `Bars ${from}–${to}`; }

// tempo (seconds per bar) of bar n (1-based) as currently rendered
function barBpm(n) { const bt = currentSong.barTimes; return Math.round(240 / (bt[n] - bt[n - 1])); }
function updateTempoHint() {
  if (!currentSong) return;
  const h = $("tempoHint");
  const v = Math.round(Number($("songBpm").value));
  const [from, to] = lockRange();
  const whole = from === 1 && to === currentSong.meta.bars;
  let msg = "";
  if ($("tempoLock").checked) {
    msg = whole ? "All bars are selected, so locking them leaves nothing to change — select fewer bars."
      : `${barsText(from, to)} keep${from === to ? "s" : ""} ${barBpm(from)} bpm; the rest of the song plays at ${v} bpm.`;
  }
  h.textContent = msg;
  h.classList.toggle("hidden", !msg);
  $("applyTempo").disabled = $("tempoLock").checked && whole;
}
$("songBpm").oninput = updateTempoHint;
$("tempoLock").onchange = updateTempoHint;
$("resetEdit").onclick = () => editSong({ transpose: 0, transposeLock: null, bpm: currentSong.meta.origBpm || currentSong.meta.bpm });

function instrumentKeep() {
  const parts = [...document.querySelectorAll("input[name=ilock]:checked:not(:disabled)")].map((c) => c.value);
  if (!parts.length) return null;
  const [from, to] = lockRange();
  const whole = from === 1 && to === currentSong.meta.bars;
  return whole ? { parts } : { parts, from, to };
}
function updateIlockHint() {
  if (!currentSong) return;
  const keep = instrumentKeep();
  let msg = "";
  if (keep) {
    const names = keep.parts.join(", ");
    msg = keep.from
      ? `${names} keep${keep.parts.length === 1 ? "s" : ""} the current instrument in ${barsText(keep.from, keep.to).toLowerCase()} and switch${keep.parts.length === 1 ? "es" : ""} to the selected one elsewhere — a mid-song instrument change.`
      : `${names} keep${keep.parts.length === 1 ? "s" : ""} the current instrument; the other parts change to the selection on the left.`;
    msg = msg[0].toUpperCase() + msg.slice(1);
  }
  $("ilockHint").textContent = msg;
  $("ilockHint").classList.toggle("hidden", !msg);
}
for (const c of document.querySelectorAll("input[name=ilock]")) c.onchange = updateIlockHint;

$("reinstrument").onclick = async () => {
  if (!currentSong) return;
  const id = currentSong.meta.id;
  player.stop();
  $("reinstrument").disabled = true;
  try {
    const ins = selectedInstruments();
    // extra parts take the instrument of the same-numbered extra row on the left
    // when it has the same role
    for (const p of currentSong.parts) {
      const i = p.kind.startsWith("extra") ? Number(p.kind.slice(5)) - 1 : -1;
      if (i >= 0 && extras[i] && extras[i].role === p.role) ins[p.kind] = extras[i].program;
    }
    await api(`/api/songs/${id}/instruments`, { method: "POST", body: JSON.stringify({ instruments: ins, keep: instrumentKeep() }) });
    player.loadedFor = null;
    await refreshLibrary();
    await loadSong(id);
  } catch (e) {
    alert(e.message);
  } finally {
    $("reinstrument").disabled = false;
  }
};
window.addEventListener("resize", () => currentSong && roll.render(currentSong));

// ---- sounds (default online FluidR3, or uploaded SoundFonts) -------------------

async function refreshSoundFonts(select) {
  const list = await api("/api/soundfonts");
  const sel = $("sound");
  const want = select ?? store.get("sound") ?? "";
  sel.innerHTML = "";
  sel.add(new Option("FluidR3 GM (online, default)", ""));
  for (const f of list) sel.add(new Option(`${f.name} · ${(f.size / 1048576).toFixed(1)} MB`, f.name));
  sel.value = list.some((f) => f.name === want) ? want : "";
  await chooseSound();
}

async function chooseSound() {
  const name = $("sound").value;
  store.set("sound", name);
  $("sfDelete").classList.toggle("hidden", !name);
  $("sfStatus").textContent = name ? "drums and every instrument come from this SoundFont" : "";
  await player.setSource(name);
}

$("sound").onchange = chooseSound;
$("sfUpload").onclick = () => $("sfFile").click();
$("sfFile").onchange = () => {
  const f = $("sfFile").files[0];
  if (!f) return;
  const name = f.name.replace(/[^A-Za-z0-9 ._()+-]/g, "_").replace(/^[^A-Za-z0-9]+/, "");
  const xhr = new XMLHttpRequest();
  xhr.open("POST", `/api/soundfonts?name=${encodeURIComponent(name)}`);
  xhr.upload.onprogress = (e) => {
    if (e.lengthComputable) $("sfStatus").textContent = `uploading ${name}: ${Math.round((100 * e.loaded) / e.total)}%`;
  };
  xhr.onload = () => {
    let msg = "";
    try { msg = JSON.parse(xhr.responseText).error || ""; } catch { /* not JSON */ }
    if (xhr.status !== 200) { $("sfStatus").textContent = "upload failed: " + (msg || xhr.statusText); return; }
    refreshSoundFonts(name);
  };
  xhr.onerror = () => ($("sfStatus").textContent = "upload failed: network error");
  $("sfUpload").disabled = true;
  xhr.onloadend = () => { $("sfUpload").disabled = false; $("sfFile").value = ""; };
  xhr.send(f);
};
$("sfDelete").onclick = async () => {
  const name = $("sound").value;
  if (!name || !confirm(`Delete the SoundFont “${name}” from the server?`)) return;
  await api(`/api/soundfonts/${encodeURIComponent(name)}`, { method: "DELETE" });
  if (player.sp) player.sp.bank = null;
  refreshSoundFonts("");
};

// ---- library -------------------------------------------------------------------

async function refreshLibrary() {
  const list = await api("/api/songs");
  const ul = $("library");
  ul.innerHTML = "";
  if (!list.length) ul.innerHTML = '<li class="muted">No songs yet.</li>';
  for (const m of list) {
    const li = document.createElement("li");
    li.dataset.id = m.id;
    li.classList.toggle("current", currentSong?.meta.id === m.id);
    const when = new Date(m.created).toLocaleString();
    li.innerHTML = `<span class="badge"></span><div class="lt"><div></div><div></div></div><button class="ghost" title="Delete">✕</button>`;
    li.querySelector(".badge").textContent = m.rating > 0 ? "👍" : m.rating < 0 ? "👎" : m.parent ? "↳" : "";
    li.querySelector(".lt div:first-child").textContent = m.title;
    li.querySelector(".lt div:last-child").textContent =
      `${fmtTime(m.seconds)} · ${m.bpm} bpm · score ${m.score.total.toFixed(0)} · ${when}`;
    li.onclick = () => loadSong(m.id);
    li.querySelector("button").onclick = async (e) => {
      e.stopPropagation();
      if (!confirm(`Delete “${m.title}”?`)) return;
      await api(`/api/songs/${m.id}`, { method: "DELETE" });
      if (currentSong?.meta.id === m.id) { player.stop(); currentSong = null; $("song").classList.add("hidden"); }
      refreshLibrary();
    };
    ul.append(li);
  }
}

window.addEventListener("hashchange", () => {
  const m = location.hash.match(/^#song=([\w-]+)$/);
  if (m && (!currentSong || currentSong.meta.id !== m[1])) loadSong(m[1]);
});

initControls().then(refreshLibrary).then(refreshTaste).then(() => refreshSoundFonts()).then(() => {
  const m = location.hash.match(/^#song=([\w-]+)$/);
  if (m) return loadSong(m[1]);
}).catch((e) => {
  document.body.insertAdjacentHTML("afterbegin", `<p style="color:var(--playhead);padding:16px">Could not load: ${e.message}</p>`);
});
