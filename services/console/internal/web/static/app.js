// The console's page. It is handed the picture - once, then again whenever
// it changes - and draws it. It asks for nothing and sends nothing.
//
// Every word that came from a player or a catalogue is put on the page as
// text, never as markup.

import { render as renderStrip, validate } from "./effects.js";

const LEDS = 60; // CONFIG_LP_LED_COUNT's default; the picture, not a count of the real strip
const $ = (id) => document.getElementById(id);

const PLAYERS = { itunes: "iTunes", music: "Music", spotify: "Spotify", musicbee: "MusicBee", tidal: "TIDAL" };
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
const clock = (ms) => `${Math.floor(ms / 60000)}:${String(Math.floor(ms / 1000) % 60).padStart(2, "0")}`;
const coverURL = (sha) => "api/cover/" + encodeURIComponent(sha);

// --- what is playing -------------------------------------------------------------

function drawNow(page) {
  const now = page.current;
  const track = now && now.track;
  const live = now && now.state !== "stopped" && track;
  const img = $("cover-img");

  document.querySelector(".now").classList.toggle("idle", !live);

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
  $("album").textContent = [track.album, track.duration_ms ? clock(track.duration_ms) : ""].filter(Boolean).join(" · ");
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

    what.textContent =
      `${scene.effect} · brightness ${Math.round(scene.brightness * 100)}%` +
      (scene.effect === "solid" ? "" : ` · one cycle every ${(6 - 5 * scene.speed).toFixed(1)} s`);
    card.canvas.setAttribute(
      "aria-label",
      `${stand.id} showing ${scene.effect} in ${scene.palette.map((s) => hex(s.rgb)).join(", ")}`,
    );

    // Breathe and solid light the dominant colour alone. Marking it says
    // which of these is on the strip without another sentence.
    const onlyFirst = scene.effect !== "sweep";
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
    const pixels = renderStrip(scene, still.matches ? (6000 - 5000 * scene.speed) / 4 : t, LEDS);
    const pitch = width / LEDS, gap = Math.min(3, pitch * 0.22);
    const top = height * 0.28, tall = height * 0.44;
    for (let i = 0; i < LEDS; i++) {
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
  drawNow(page);
  drawStands(page);
  drawSources(page);
  drawRecent(page);
  drawFrame(page);
}

// --- staying current -------------------------------------------------------------

function listen() {
  const live = $("live");
  const stream = new EventSource("api/events");
  stream.onopen = () => { live.className = "live on"; live.textContent = "live"; };
  stream.onmessage = (event) => {
    try {
      draw(JSON.parse(event.data));
    } catch (problem) {
      console.error("could not draw what was sent", problem);
    }
  };
  // The browser reconnects by itself. What is on the page stays, marked as
  // possibly old, which is the truth.
  stream.onerror = () => { live.className = "live off"; live.textContent = "lost the console; trying again"; };
}

listen();
requestAnimationFrame(paint);
