// The console's page. It is handed the picture - once, then again whenever
// it changes - and draws it. It asks for nothing and sends nothing.
//
// Every word that came from a player or a catalogue is put on the page as
// text, never as markup.

import { label, position, start } from "./clock.js";
import { periodMs, render as renderStrip, validate } from "./effects.js";

// How many LEDs the strip is drawn with. The page is told; this is only what
// is drawn before it has been.
let leds = 18;
const $ = (id) => document.getElementById(id);

const PLAYERS = { itunes: "iTunes", music: "Music", spotify: "Spotify", musicbee: "MusicBee", tidal: "TIDAL", turntable: "Turntable" };
const playerName = (p) => (p && (PLAYERS[p.player] || p.player || p.source)) || "";

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function pill(label, up) {
  // up: true, false, or undefined for "has not said".
  const node = el("span", "pill" + (up === true ? " up" : up === false ? " down" : ""), label);
  node.title = up === true ? "online" : up === false ? "offline" : "not heard from yet";
  return node;
}

const hex = (rgb) => "#" + rgb.map((c) => c.toString(16).padStart(2, "0")).join("");
const coverURL = (sha) => "api/cover/" + encodeURIComponent(sha);

// --- what is playing -------------------------------------------------------------

// What the playhead counts from, set each time a picture arrives.
let playhead = null;

// tick writes where playback is now. It runs a few times a second; the
// seconds only change once in each.
function tick() {
  const ms = position(playhead, performance.now());
  const row = $("playhead");
  if (ms === null) {
    row.hidden = true;
    return;
  }
  row.hidden = false;
  const text = label(ms);
  if ($("elapsed").textContent !== text) $("elapsed").textContent = text;
  const bar = $("bar");
  if (playhead.duration) {
    $("bar-fill").style.width = `${Math.min(100, (ms / playhead.duration) * 100)}%`;
    bar.hidden = false;
    bar.setAttribute("aria-valuemax", String(Math.round(playhead.duration / 1000)));
    bar.setAttribute("aria-valuenow", String(Math.round(ms / 1000)));
    bar.setAttribute("aria-valuetext", `${text} of ${label(playhead.duration)}`);
    $("length").textContent = label(playhead.duration);
  } else {
    bar.hidden = true;
    $("length").textContent = "";
  }
}
setInterval(tick, 250);

function drawNow(page) {
  const now = page.current;
  const track = now && now.track;
  const live = now && now.state !== "stopped" && track;
  const img = $("cover-img");

  document.querySelector(".now").classList.toggle("idle", !live);
  document.querySelector(".now").classList.toggle("paused", Boolean(live) && now.state === "paused");
  playhead = start(now, page.now, performance.now());
  tick();

  if (!live) {
    $("where").textContent = "";
    $("title").textContent = page.current ? "Nothing is playing" : "Listening…";
    $("artist").textContent = "";
    $("album").textContent = "";
    $("cover-note").textContent = page.current ? "" : "Nothing has been heard from nowplayd yet.";
    img.hidden = true;
    $("cover-empty").textContent = "";
    document.title = "EdS";
    return;
  }

  $("where").textContent = (now.state === "paused" ? "Paused on " : "Playing on ") + playerName(now);
  $("title").textContent = track.title || "(untitled)";
  $("artist").textContent = track.artist || track.album_artist || "";
  $("album").textContent = track.album || "";
  document.title = `${track.title || "EdS"} · EdS`;

  // The cover held is the one the stand is lit from. It is this track's only
  // if this track names it; a source reported without its cover names none.
  if (now.cover && now.cover === page.lit_cover) {
    const url = coverURL(now.cover);
    if (img.getAttribute("src") !== url) img.src = url;
    img.alt = `Cover of ${track.album || track.title || "this track"}`;
    img.hidden = false;
    $("cover-note").textContent =
      now.cover_from === "player" || !now.cover_from
        ? "Cover from the player's own library."
        : now.cover_from === "discogs"
          ? "The sleeve of the record in the owner's collection. Data provided by Discogs."
          : `No cover in the library; this one was found in ${now.cover_from}.`;
  } else {
    img.hidden = true;
    img.removeAttribute("src");
    $("cover-empty").textContent = now.cover ? "cover on its way" : "no cover";
    $("cover-note").textContent = now.cover
      ? ""
      : `${playerName(now)} is reported without its cover, so the stand does not follow it. ` +
        "It holds the last cover it was lit from.";
  }
}

// --- stands ----------------------------------------------------------------------

// Canvases are kept between updates, by stand, so an update does not restart
// an animation or flash the strip.
const strips = new Map(); // id -> { node, canvas, scene }

function standCard(id) {
  const node = el("article", "stand");
  const head = el("div", "stand-head");
  head.append(el("h2", "", id), el("span", "pill"), el("span", "what"));
  const canvas = el("canvas", "strip");
  canvas.setAttribute("role", "img");
  const foot = el("div", "stand-foot");
  foot.append(el("ul", "swatches"), el("div", "from"));
  node.append(head, canvas, foot, el("p", "fine"));
  return { node, canvas, scene: null };
}

function drawStands(page) {
  const host = $("stands");
  const seen = new Set();

  for (const stand of page.stands) {
    seen.add(stand.id);
    let card = strips.get(stand.id);
    if (!card) {
      card = standCard(stand.id);
      strips.set(stand.id, card);
      host.append(card.node);
    }
    const scene = validate(stand.scene);
    card.scene = scene;

    const online = stand.online;
    card.node.classList.toggle("away", online === false);
    card.node.querySelector(".pill").replaceWith(pill(online === false ? "offline" : online ? "online" : "unknown", online));

    const what = card.node.querySelector(".what");
    const swatches = card.node.querySelector(".swatches");
    const from = card.node.querySelector(".from");
    const fine = card.node.querySelector(".fine");
    swatches.replaceChildren();
    from.replaceChildren();

    if (!scene) {
      what.textContent = "no scene sent yet";
      fine.textContent = "";
      card.canvas.setAttribute("aria-label", `${stand.id}: no scene`);
      continue;
    }

    what.textContent = `${scene.effect} · brightness ${Math.round(scene.brightness * 100)}%` + pace(scene);
    card.canvas.setAttribute(
      "aria-label",
      `${stand.id} showing ${scene.effect} in ${scene.palette.map((s) => hex(s.rgb)).join(", ")}`,
    );

    // Breathe and solid light the dominant colour alone. Marking it says
    // which of these is on the strip without another sentence.
    const onlyFirst = scene.effect === "solid" || scene.effect === "breathe";
    scene.palette.forEach((sw, i) => {
      const item = el("li", "swatch" + (onlyFirst && i === 0 ? " lit" : ""));
      const chip = el("i");
      chip.style.background = hex(sw.rgb);
      item.append(chip, el("code", "", hex(sw.rgb)), el("span", "", `${Math.round(sw.weight * 100)}%`));
      swatches.append(item);
    });

    if (page.lit_cover) {
      const thumb = el("img");
      thumb.src = coverURL(page.lit_cover);
      thumb.alt = "";
      from.append(thumb, el("span", "", "Lit from this cover."));
    }

    const notes = [];
    if (onlyFirst && scene.palette.length > 1) {
      notes.push(`In ${scene.effect}, the strip is the first colour alone; the others are what the cover also held.`);
    }
    if (online === false) notes.push("The stand is offline. This is the scene waiting for it, not one it is showing.");
    else if (stand.applied && stand.applied.effect !== scene.effect) {
      notes.push(`The stand last reported showing ${stand.applied.effect}; it may not have taken this scene yet.`);
    }
    notes.push("Drawn from the scene as sent, the way the stand animates it. The stand's own LEDs are not measured.");
    fine.textContent = notes.join(" ");
  }

  for (const [id, card] of strips) {
    if (!seen.has(id)) {
      card.node.remove();
      strips.delete(id);
    }
  }

  // The page takes its accent from the first stand that has a scene.
  const first = [...strips.values()].find((c) => c.scene);
  if (first) document.documentElement.style.setProperty("--accent", accent(first.scene.palette[0].rgb));
}

// pace says how long the effect takes to come round, in the effect's own
// terms: a breath, a lap, a turn through every colour.
function pace(scene) {
  const seconds = (ms) => `${(ms / 1000).toFixed(1)} s`;
  const cycle = periodMs(scene.speed);
  switch (scene.effect) {
    case "breathe": return ` · a breath every ${seconds(cycle)}`;
    case "sweep": return ` · once round every ${seconds(cycle)}`;
    case "chase": return scene.palette.length > 1 ? ` · a lap every ${seconds(cycle * 2)}` : "";
    case "morph": return scene.palette.length > 1 ? ` · through every colour in ${seconds(cycle * 2 * scene.palette.length)}` : "";
    default: return "";
  }
}

// accent is a colour from the scene made fit to be text on a dark page: a
// cover's dominant colour is often nearly black.
function accent(rgb) {
  const peak = Math.max(...rgb, 1);
  const lift = Math.max(1, 190 / peak);
  return hex(rgb.map((c) => Math.min(255, Math.round(c * lift + 18))));
}

const still = window.matchMedia("(prefers-reduced-motion: reduce)");

function paint(t) {
  for (const { canvas, scene } of strips.values()) {
    const width = canvas.clientWidth, height = canvas.clientHeight;
    if (!width || !height) continue;
    const ratio = window.devicePixelRatio || 1;
    if (canvas.width !== Math.round(width * ratio) || canvas.height !== Math.round(height * ratio)) {
      canvas.width = Math.round(width * ratio);
      canvas.height = Math.round(height * ratio);
    }
    const ctx = canvas.getContext("2d");
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    ctx.clearRect(0, 0, width, height);
    if (!scene) continue;

    // Without motion the strip is shown at one moment: the top of a breath.
    const pixels = renderStrip(scene, still.matches ? periodMs(scene.speed) / 4 : t, leds);
    const pitch = width / leds, gap = Math.min(6, pitch * 0.18);
    const top = height * 0.28, tall = height * 0.44;
    for (let i = 0; i < leds; i++) {
      const colour = `rgb(${pixels[i][0]},${pixels[i][1]},${pixels[i][2]})`;
      ctx.shadowColor = colour;
      ctx.shadowBlur = height * 0.34;
      ctx.fillStyle = colour;
      ctx.beginPath();
      ctx.roundRect(i * pitch + gap / 2, top, pitch - gap, tall, 3);
      ctx.fill();
    }
    ctx.shadowBlur = 0;
  }
  if (!still.matches) requestAnimationFrame(paint);
}
still.addEventListener("change", () => requestAnimationFrame(paint));

// --- the lists -------------------------------------------------------------------

function drawSources(page) {
  const rows = page.sources.map((source) => {
    const row = el("li");
    const now = source.now;
    row.append(el("span", "main", playerName(now) || PLAYERS[source.id] || source.id));
    row.append(pill(source.online === false ? "offline" : source.online ? "online" : "unknown", source.online));
    let line = "nothing reported yet";
    if (now) {
      const track = now.track;
      line = track ? `${now.state}: ${[track.artist, track.title].filter(Boolean).join(" – ")}` : "nothing loaded";
    }
    row.append(el("span", "sub", line));
    return row;
  });
  $("sources").replaceChildren(...(rows.length ? rows : [el("li", "empty", "No player has reported yet.")]));
}

function drawRecent(page) {
  const rows = page.recent.map((play) => {
    const row = el("li");
    const at = new Date(play.at);
    row.append(el("time", "lead", at.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })));
    row.lastChild.dateTime = play.at;
    row.append(el("span", "main", [play.track.artist, play.track.title].filter(Boolean).join(" – ") || "(untitled)"));
    row.append(el("span", "sub", [playerName(play), play.track.album].filter(Boolean).join(" · ")));
    return row;
  });
  $("recent").replaceChildren(...(rows.length ? rows : [el("li", "empty", "Nothing has started playing since the console did.")]));
}

function drawFrame(page) {
  const pills = [];
  const word = (w) => (w === "online" ? true : w === "offline" ? false : undefined);
  pills.push(pill("nowplayd", word(page.services.nowplayd)), pill("lightd", word(page.services.lightd)));
  for (const s of page.sources) pills.push(pill(PLAYERS[s.id] || s.id, s.online === null ? undefined : s.online));
  for (const s of page.stands) pills.push(pill(s.id, s.online === null ? undefined : s.online));
  // A beacon that is off is not a part that is down: it is a switch that is
  // off, and lit only when it is on.
  for (const b of page.beacons) {
    const node = pill(b.id, b.online === true ? true : undefined);
    node.title = b.online === true ? "switched on" : b.online === false ? "switched off" : "not heard from yet";
    pills.push(node);
  }
  $("pills").replaceChildren(...pills.map((p) => { const li = el("li"); li.append(p); return li; }));

  const links = [];
  for (const [label, url] of [["Events in Grafana", page.links.dashboard], ["Decisions and records", page.links.docs]]) {
    if (typeof url === "string" && /^https?:\/\//.test(url)) {
      const a = el("a", "", label);
      a.href = url;
      a.rel = "noreferrer";
      links.push(a);
    }
  }
  $("links").replaceChildren(...links);
  $("version").textContent = page.version ? `console ${page.version}` : "";
}

function draw(page) {
  if (Number.isInteger(page.strip_leds) && page.strip_leds > 0 && page.strip_leds <= 300) leds = page.strip_leds;
  drawNow(page);
  drawStands(page);
  drawSources(page);
  drawRecent(page);
  drawFrame(page);
}

// --- staying current -------------------------------------------------------------

// The console sends a beat every 15 seconds when it has nothing else to say.
// A stream that has been silent for longer than a few of them is dead, though
// the browser may not know it: a laptop that slept, or a connection dropped
// somewhere in between, leaves a stream that looks open and says nothing.
const QUIET_TOO_LONG_MS = 40000;

let stream = null;
let heard = 0;

function listen() {
  const live = $("live");
  if (stream) stream.close();
  heard = performance.now();
  stream = new EventSource("api/events");
  stream.onopen = () => { heard = performance.now(); live.className = "live on"; live.textContent = "live"; };
  stream.onmessage = (event) => {
    heard = performance.now();
    try {
      draw(JSON.parse(event.data));
    } catch (problem) {
      console.error("could not draw what was sent", problem);
    }
  };
  stream.addEventListener("beat", () => { heard = performance.now(); });
  // The browser reconnects by itself. What is on the page stays, marked as
  // possibly old, which is the truth.
  stream.onerror = () => { live.className = "live off"; live.textContent = "lost the console; trying again"; };
}

function checkStream() {
  if (performance.now() - heard < QUIET_TOO_LONG_MS) return;
  const live = $("live");
  live.className = "live off";
  live.textContent = "nothing heard for a while; reconnecting";
  listen();
}

listen();
setInterval(checkStream, 5000);
// Coming back to the tab, or back onto the network, is when a dead stream is
// most likely and least welcome: check at once and do not wait for the timer.
document.addEventListener("visibilitychange", () => { if (!document.hidden) checkStream(); });
window.addEventListener("online", listen);
requestAnimationFrame(paint);
