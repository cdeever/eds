// The console's page. It is handed the picture - once, then again whenever
// it changes - and draws it. What it sends is two settings: which sources
// light the stand (ADR-0039), and whether Spotify is followed (ADR-0051).
//
// Every word that came from a player or a catalog is put on the page as
// text, never as markup.

import { label, position, start } from "./clock.js";
import { periodMs, render as renderStrip, validate } from "./effects.js";

// How many LEDs the strip is drawn with. The page is told; this is only what
// is drawn before it has been.
let leds = 18;
const $ = (id) => document.getElementById(id);

const PLAYERS = { itunes: "iTunes", music: "Music", spotify: "Spotify", musicbee: "MusicBee", tidal: "TIDAL", turntable: "Turntable" };
const playerName = (p) => (p && (PLAYERS[p.player] || p.player || p.source)) || "";
// A source's pill in the header is its driver's presence, not the player's:
// the turntable's says the sound card is being heard, not that the turntable
// is on (ADR-0034). The source is still "turntable" everywhere else.
const DRIVERS = { ...PLAYERS, turntable: "Song detector" };

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

// What each presence is called, and said on hovering (ADR-0034). A stored
// online is only a claim: the broker keeps one after its part is gone.
const PRESENCE = {
  online: ["online", "heard from in the last ninety seconds"],
  claimed: ["claimed", "the broker holds an online from it, and it has not been heard since the console started"],
  offline: ["offline", "said it is offline"],
  unheard: ["not heard", "not heard from for ninety seconds"],
};

function presencePill(label, state) {
  const node = pill(label, state === "online" ? true : state === "offline" ? false : undefined);
  node.title = (PRESENCE[state] || [, "not heard from yet"])[1];
  return node;
}
const presenceWord = (state) => (PRESENCE[state] || ["unknown"])[0];

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
    img.removeAttribute("src");
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
    card.node.classList.toggle("away", stand.presence !== "online");
    card.node.querySelector(".pill").replaceWith(presencePill(presenceWord(stand.presence), stand.presence));

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

    // Breathe and solid light the dominant color alone. Marking it says
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
      notes.push(`In ${scene.effect}, the strip is the first color alone; the others are what the cover also held.`);
    }
    if (online === false) notes.push("The stand is offline. This is the scene waiting for it, not one it is showing.");
    else if (stand.presence !== "online") notes.push("The stand has not been heard from lately. This is the scene waiting for it, and it may not be showing it.");
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

// pace says how long the effect takes to come around, in the effect's own
// terms: a breath, a lap, a turn through every color.
function pace(scene) {
  const seconds = (ms) => `${(ms / 1000).toFixed(1)} s`;
  const cycle = periodMs(scene.speed);
  switch (scene.effect) {
    case "breathe": return ` · a breath every ${seconds(cycle)}`;
    case "sweep": return ` · once around every ${seconds(cycle)}`;
    case "scanner": return ` · there and back every ${seconds(cycle)}`;
    case "chase": return scene.palette.length > 1 ? ` · a lap every ${seconds(cycle * 2)}` : "";
    case "morph": return scene.palette.length > 1 ? ` · through every color in ${seconds(cycle * 2 * scene.palette.length)}` : "";
    default: return "";
  }
}

// accent is a color from the scene made fit to be text on a dark page: a
// cover's dominant color is often nearly black.
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
      const color = `rgb(${pixels[i][0]},${pixels[i][1]},${pixels[i][2]})`;
      ctx.shadowColor = color;
      ctx.shadowBlur = height * 0.34;
      ctx.fillStyle = color;
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
    row.append(presencePill(presenceWord(source.presence), source.presence));
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

// --- the two settings (ADR-0039, ADR-0051) -------------------------------------

// How long a switch waits to hear its setting come back from the broker.
const ECHO_MS = 5000;

// A setting the page can switch: shown as the bus last said it, switched by
// a POST to the console. What is shown is what comes back on the bus, not
// what was asked: the setting reaches whoever obeys it the same way, so
// hearing it is the proof.
function settingSwitch({ section, word, button, note, path, field, words, describe }) {
  let shown = null; // the word in force, as last heard
  let wanted = null; // the word asked for and not yet heard back
  let wantedAt = 0;

  async function flip() {
    wanted = shown === words[1] ? words[0] : words[1];
    wantedAt = performance.now();
    $(button).disabled = true;
    $(note).textContent = "switching…";
    try {
      const resp = await fetch(path, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ [field]: wanted }),
      });
      if (!resp.ok) throw new Error(`${resp.status} ${(await resp.text()).trim()}`);
    } catch (problem) {
      wanted = null;
      $(button).disabled = false;
      $(note).textContent = `Not switched: ${problem.message}`;
    }
  }
  $(button).addEventListener("click", flip);

  setInterval(() => {
    if (wanted && performance.now() - wantedAt > ECHO_MS) {
      wanted = null;
      $(button).disabled = false;
      $(note).textContent =
        "Sent, and not heard back from the broker. The console's account may not be allowed to change the setting.";
    }
  }, 1000);

  // draw shows the word in force. aside is a note of the page's own, shown
  // when nothing is being switched.
  return function draw(value, aside = "") {
    shown = value;
    const d = describe(value);
    $(section).classList.toggle("marked", d.marked);
    $(word).textContent = d.word;
    const b = $(button);
    b.hidden = false;
    b.textContent = d.button;
    b.title = d.title;
    if (wanted && value === wanted) wanted = null;
    if (!wanted) $(note).textContent = aside;
    b.disabled = wanted !== null;
  };
}

// Which sources light the stand (ADR-0039). Every source is the setting
// someone left on, and is marked so it is not missed.
const drawStandSetting = settingSwitch({
  section: "stand-sources", word: "stand-word", button: "stand-switch", note: "stand-note",
  path: "api/stand", field: "sources", words: ["records", "all"],
  describe: (v) => v === "all"
    ? { marked: true, word: "lit for every source", button: "Records only",
        title: "Light the stand for records only, as it is by default" }
    : { marked: false, word: "lit for records", button: "Every source",
        title: "Light the stand for iTunes and Spotify too, until it is switched back" },
});

// Whether Spotify's driver runs (ADR-0051). Off is plain, not a fault: it is
// the default, and asks Spotify nothing.
const drawSpotifySetting = settingSwitch({
  section: "spotify-switch", word: "spotify-word", button: "spotify-flip", note: "spotify-note",
  path: "api/spotify", field: "driver", words: ["off", "on"],
  describe: (v) => v === "on"
    ? { marked: false, word: "followed", button: "Turn off",
        title: "Stop asking Spotify what is playing" }
    : { marked: false, word: "off: Spotify is asked nothing", button: "Turn on",
        title: "Follow Spotify until it is turned off; it is asked what is playing every few seconds" },
});

// Switched on and still not online after this long is a driver that could
// not start: the agent's own presence is believed for ninety seconds.
const SPOTIFY_START_MS = 90000;
let spotifyOnSince = null;

function drawSettings(page) {
  drawStandSetting(page.stand_sources === "all" ? "all" : "records");

  const on = page.spotify_driver === "on";
  const source = page.sources.find((s) => s.id === "spotify");
  let aside = "";
  if (!on) {
    spotifyOnSince = null;
  } else if (source && source.presence === "online") {
    spotifyOnSince = null;
  } else {
    spotifyOnSince ??= performance.now();
    if (performance.now() - spotifyOnSince > SPOTIFY_START_MS) {
      aside = "Switched on, and Spotify's driver has not come online. It may not be in the agent's NP_DRIVERS, " +
        "or may need its authorization renewed: the agent's log says which.";
    }
  }
  drawSpotifySetting(on ? "on" : "off", aside);
}

// What a beacon switches, from its name: turntable-power-01 is the
// turntable's. The console is not told which source a beacon speaks for.
function switched(id) {
  const m = /^(.+)-power(-\d+)?$/.exec(id);
  return m ? m[1].charAt(0).toUpperCase() + m[1].slice(1).replace(/-/g, " ") : id;
}

function drawFrame(page) {
  const services = ["nowplayd", "lightd"].map((name) => presencePill(name, page.services[name]));
  const sources = page.sources.map((s) => presencePill(DRIVERS[s.id] || s.id, s.presence));
  const devices = [...page.stands, ...page.screens].map((d) => presencePill(d.id, d.presence));
  // A beacon that is off is not a part that is down: it is a switch that is
  // off, and lit only when it is on.
  const power = page.beacons.map((b) => {
    const on = b.presence === "online";
    const node = pill(`${switched(b.id)}: ${on ? "on" : b.presence === "offline" ? "off" : "?"}`, on ? true : undefined);
    node.title = `${b.id}: ` + (on ? "switched on" : b.presence === "offline" ? "switched off" : (PRESENCE[b.presence] || [, "not heard from yet"])[1]);
    return node;
  });
  const kinds = [["Services", services], ["Sources", sources], ["Devices", devices], ["Power", power]];
  $("parts").replaceChildren(...kinds.filter(([, pills]) => pills.length).map(([name, pills]) => {
    const row = el("div", "kind");
    const list = el("ul", "pills");
    list.setAttribute("aria-label", name);
    list.append(...pills.map((p) => { const li = el("li"); li.append(p); return li; }));
    row.append(el("span", "kind-name", name), list);
    return row;
  }));

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
  drawSettings(page);
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
