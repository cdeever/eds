// Where in a track playback is, worked out from what the bus said.
//
// A state on the bus carries where playback was (position_ms) at the moment
// the track or the play state last changed (changed_at), by the sender's
// clock. Nothing is sent while a track simply plays, so the time since is
// added here: the server says what time it was when it sent the picture, and
// the page counts on from when the picture arrived. The viewer's own clock is
// never compared with anyone else's, so a laptop five minutes wrong shows the
// right time.
//
// What this cannot know is a seek. Dragging the playhead changes neither the
// track nor the play state, so nothing is reported, and the count runs on
// from where it was until the next pause, resume or track.
//
// Pure: no page, no clock of its own.

// start is what to count from, taken once when a picture arrives. `received`
// is the page's own monotonic time at that moment. Null when there is
// nothing to show a position for.
export function start(current, serverNow, received) {
  if (!current || !current.track || current.state === "stopped") return null;
  if (typeof current.position_ms !== "number" || !(current.position_ms >= 0)) return null;

  const duration = current.track.duration_ms > 0 ? current.track.duration_ms : 0;
  let at = current.position_ms;
  const playing = current.state === "playing";
  if (playing) {
    const changed = Date.parse(current.changed_at);
    const now = Date.parse(serverNow);
    // Without both instants the position is only where it was, and counting
    // on from it would be a guess presented as a clock.
    if (Number.isNaN(changed) || Number.isNaN(now)) return null;
    // A sender's clock slightly ahead of the server's is not time travel.
    at += Math.max(0, now - changed);
  }
  return { at, playing, duration, received };
}

// position is where playback is at the page's monotonic time `t`, in
// milliseconds, never past the end of the track.
export function position(from, t) {
  if (!from) return null;
  let ms = from.at + (from.playing ? Math.max(0, t - from.received) : 0);
  if (from.duration && ms > from.duration) ms = from.duration;
  return ms;
}

// label is "2:06", or "1:02:06" for something an hour long.
export function label(ms) {
  const total = Math.floor(ms / 1000);
  const h = Math.floor(total / 3600), m = Math.floor(total / 60) % 60, s = total % 60;
  const two = (n) => String(n).padStart(2, "0");
  return h ? `${h}:${two(m)}:${two(s)}` : `${m}:${two(s)}`;
}
