/* ===========================================================================
   StupidChess frontend
   Vanilla ES2020+. No build step, no libraries, fully offline.

   Sections:
     1. State
     2. Constants & small helpers
     3. Rendering  (board, pieces, move list, info, eval bar, status)
     4. Board interaction  (select / move / drag / promotion)
     5. Networking  (SSE in, POST actions out)
     6. Event handlers & wiring

   The server is authoritative. We POST actions and then wait for the next
   `state` event to re-render. We never trust optimistic local state for legality.
   =========================================================================== */

"use strict";

/* ===========================================================================
   1. STATE
   =========================================================================== */

// Single source of truth, replaced wholesale from each `state` event.
const game = {
  fen: "8/8/8/8/8/8/8/8 w - - 0 1",
  turn: "w",
  dests: {},
  lastMove: null,
  check: false,
  inProgress: false,
  result: null,
  reason: null,
  moves: [],
  players: { white: "human", black: "human" },
  thinking: null,
  mode: "auto",
};

// Frontend-only / derived UI state.
const ui = {
  engines: [],          // engine descriptors {id, name, description} from the `engines` event
  engineById: {},       // id -> descriptor, for quick lookup
  flipped: false,       // board orientation, owned entirely by the frontend
  selected: null,       // currently selected from-square (e.g. "e2") or null
  drag: null,           // active drag descriptor or null
  pendingPromo: null,   // { from, to, color } while picker is open
  lastInfo: { w: null, b: null }, // last `info` seen per side
};

/* ===========================================================================
   2. CONSTANTS & HELPERS
   =========================================================================== */

const FILES = ["a", "b", "c", "d", "e", "f", "g", "h"];
const RANKS = ["1", "2", "3", "4", "5", "6", "7", "8"];

// Pieces are SVGs (the cburnett set) served from /pieces/<color><LETTER>.svg,
// e.g. /pieces/wN.svg. Swap the whole set by replacing the files in web/static/pieces.
function pieceSrc(p) {
  return "/pieces/" + p.color + p.type.toUpperCase() + ".svg";
}

const $ = (sel) => document.querySelector(sel);
const el = (tag, cls) => {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  return e;
};

// Map a piece char (FEN) to {type, color}. Uppercase = white.
function pieceFromChar(ch) {
  const isWhite = ch === ch.toUpperCase();
  return { type: ch.toLowerCase(), color: isWhite ? "w" : "b" };
}

// Parse the board portion of a FEN into an 8x8 array indexed [rank0..7][file0..7]
// where rank 0 is rank 1 ("a1") for our internal model. Returns piece chars or null.
function parseFenBoard(fen) {
  const grid = Array.from({ length: 8 }, () => new Array(8).fill(null));
  const placement = (fen || "").split(" ")[0] || "";
  const rows = placement.split("/"); // rows[0] is rank 8 (top in FEN)
  for (let r = 0; r < 8 && r < rows.length; r++) {
    const rank = 7 - r; // FEN top row is rank 8 -> internal index 7
    let file = 0;
    for (const ch of rows[r]) {
      if (file > 7) break;
      if (/\d/.test(ch)) {
        file += parseInt(ch, 10);
      } else {
        grid[rank][file] = ch;
        file++;
      }
    }
  }
  return grid;
}

const sqName = (file, rank) => FILES[file] + RANKS[rank];

// Humanize big numbers: 1234 -> "1.2k", 1_200_000 -> "1.2M".
function humanize(n) {
  if (n == null || !isFinite(n)) return "—";
  const abs = Math.abs(n);
  if (abs >= 1e9) return trimZero(n / 1e9) + "B";
  if (abs >= 1e6) return trimZero(n / 1e6) + "M";
  if (abs >= 1e3) return trimZero(n / 1e3) + "k";
  return String(n);
}
function trimZero(x) {
  return (Math.round(x * 10) / 10).toString();
}

// Format a score object {type:"cp"|"mate", value} for display.
function formatScore(score) {
  if (!score || typeof score.value !== "number") return "—";
  if (score.type === "mate") {
    const v = score.value;
    return (v < 0 ? "-M" : "M") + Math.abs(v);
  }
  // centipawns -> pawns with sign
  const pawns = score.value / 100;
  const sign = pawns > 0 ? "+" : pawns < 0 ? "" : "+"; // 0.00 shown as +0.00
  return sign + pawns.toFixed(2);
}

// Format time in ms -> "47ms" or "2.4s".
function formatTime(ms) {
  if (ms == null || !isFinite(ms)) return "—";
  if (ms < 1000) return ms + "ms";
  return trimZero(ms / 1000) + "s";
}

const sideToWord = (s) => (s === "w" ? "White" : "Black");
const playerOf = (side) => (side === "w" ? game.players.white : game.players.black);
const isHuman = (name) => !name || name === "human";

/* ===========================================================================
   2b. SOUND (Web Audio, synthesised — no audio files) & THEME
   =========================================================================== */

// Move feedback is synthesised with the Web Audio API: short percussive blips
// for a "wooden" feel, a filtered noise burst for captures, an alert pair for
// check, and a little rising motif at game end. Nothing is loaded from disk.
const Sound = (() => {
  let ctx = null;
  let on = (localStorage.getItem("sc-sound") || "on") !== "off";

  function audio() {
    if (!ctx) {
      const AC = window.AudioContext || window.webkitAudioContext;
      if (!AC) return null;
      ctx = new AC();
    }
    if (ctx.state === "suspended") ctx.resume();
    return ctx;
  }

  function blip(freq, opt) {
    opt = opt || {};
    const c = audio(); if (!c) return;
    const t = c.currentTime + (opt.at || 0);
    const dur = opt.dur || 0.08;
    const osc = c.createOscillator();
    const g = c.createGain();
    osc.type = opt.type || "triangle";
    osc.frequency.setValueAtTime(freq, t);
    g.gain.setValueAtTime(0.0001, t);
    g.gain.exponentialRampToValueAtTime(opt.gain || 0.16, t + 0.006);
    g.gain.exponentialRampToValueAtTime(0.0001, t + dur);
    osc.connect(g).connect(c.destination);
    osc.start(t);
    osc.stop(t + dur + 0.02);
  }

  function thud(opt) {
    opt = opt || {};
    const c = audio(); if (!c) return;
    const t = c.currentTime;
    const dur = opt.dur || 0.1;
    const n = Math.floor(c.sampleRate * dur);
    const buf = c.createBuffer(1, n, c.sampleRate);
    const d = buf.getChannelData(0);
    for (let i = 0; i < n; i++) d[i] = (Math.random() * 2 - 1) * (1 - i / n); // decaying noise
    const src = c.createBufferSource(); src.buffer = buf;
    const lp = c.createBiquadFilter(); lp.type = "lowpass"; lp.frequency.value = opt.filter || 1500;
    const g = c.createGain();
    g.gain.setValueAtTime(opt.gain || 0.18, t);
    g.gain.exponentialRampToValueAtTime(0.0001, t + dur);
    src.connect(lp).connect(g).connect(c.destination);
    src.start(t);
    src.stop(t + dur);
  }

  const kinds = {
    move()    { blip(210, { type: "triangle", dur: 0.07, gain: 0.15 }); },
    capture() { thud({ dur: 0.11, gain: 0.22, filter: 1300 }); blip(140, { type: "sine", dur: 0.1, gain: 0.1 }); },
    castle()  { blip(196, { type: "triangle", dur: 0.06, gain: 0.14 }); blip(247, { type: "triangle", dur: 0.06, gain: 0.12, at: 0.07 }); },
    check()   { blip(680, { type: "sine", dur: 0.09, gain: 0.13 }); blip(1015, { type: "sine", dur: 0.11, gain: 0.11, at: 0.08 }); },
    end()     { [523.25, 659.25, 783.99].forEach((f, i) => blip(f, { type: "triangle", dur: 0.16, gain: 0.12, at: i * 0.12 })); },
  };

  return {
    play(k) { if (on && kinds[k]) { try { kinds[k](); } catch (_) {} } },
    enabled() { return on; },
    set(v) { on = v; localStorage.setItem("sc-sound", v ? "on" : "off"); },
    unlock() { audio(); },
  };
})();

function countPieces(fen) {
  let c = 0;
  for (const ch of (fen || "").split(" ")[0]) if (/[a-z]/i.test(ch)) c++;
  return c;
}

// Theme (light / dark). The inline <head> script set the initial value before paint.
function currentTheme() {
  return document.documentElement.getAttribute("data-theme") === "light" ? "light" : "dark";
}
function applyTheme(theme) {
  document.documentElement.setAttribute("data-theme", theme);
  localStorage.setItem("sc-theme", theme);
  const btn = $("#btn-theme");
  if (btn) btn.textContent = theme === "dark" ? "☾ Dark" : "☀ Light";
}
function syncSoundButton() {
  const btn = $("#btn-sound");
  if (!btn) return;
  const enabled = Sound.enabled();
  btn.textContent = enabled ? "♪ Sound" : "♪ Muted";
  btn.style.opacity = enabled ? "1" : "0.5";
}

/* ===========================================================================
   3. RENDERING
   =========================================================================== */

const boardEl = $("#board");

// Build the 8x8 squares once; we reuse the DOM nodes and just update classes.
// squareEls is keyed by algebraic name -> element.
const squareEls = {};

function buildBoard() {
  boardEl.innerHTML = "";
  // Visual order: top row first. When not flipped, top is rank 8, left is file a.
  for (let visRank = 7; visRank >= 0; visRank--) {
    for (let visFile = 0; visFile < 8; visFile++) {
      const file = ui.flipped ? 7 - visFile : visFile;
      const rank = ui.flipped ? 7 - visRank : visRank;
      const name = sqName(file, rank);
      const isLight = (file + rank) % 2 === 1; // a1 (0,0) is dark
      const sq = el("div", "square " + (isLight ? "light" : "dark"));
      sq.dataset.sq = name;

      // Coordinates: files along the bottom row, ranks along the left column.
      const onBottom = visRank === 0;
      const onLeft = visFile === 0;
      if (onBottom) {
        const c = el("span", "coord file");
        c.textContent = FILES[file];
        sq.appendChild(c);
      }
      if (onLeft) {
        const c = el("span", "coord rank");
        c.textContent = RANKS[rank];
        sq.appendChild(c);
      }
      boardEl.appendChild(sq);
      squareEls[name] = sq;
    }
  }
}

// Render pieces + highlights from current game state.
function renderBoard() {
  if (Object.keys(squareEls).length === 0) buildBoard();

  const grid = parseFenBoard(game.fen);
  const interactive = boardIsInteractive();
  boardEl.classList.toggle("disabled", !interactive);

  const lastFrom = game.lastMove && game.lastMove[0];
  const lastTo = game.lastMove && game.lastMove[1];

  // Find the king square of the side to move when in check (for the red glow).
  let checkSq = null;
  if (game.check) {
    for (let r = 0; r < 8; r++) {
      for (let f = 0; f < 8; f++) {
        const ch = grid[r][f];
        if (ch && ch.toLowerCase() === "k") {
          const p = pieceFromChar(ch);
          if (p.color === game.turn) checkSq = sqName(f, r);
        }
      }
    }
  }

  const dests = (ui.selected && game.dests[ui.selected]) || [];
  const destSet = new Set(dests);

  for (const name of Object.keys(squareEls)) {
    const sq = squareEls[name];
    const file = FILES.indexOf(name[0]);
    const rank = RANKS.indexOf(name[1]);
    const ch = grid[rank][file];

    // reset state classes (keep base light/dark + coords)
    sq.classList.remove("lastmove", "selected", "check", "dest", "capture", "movable");

    if (name === lastFrom || name === lastTo) sq.classList.add("lastmove");
    if (name === checkSq) sq.classList.add("check");
    if (name === ui.selected) sq.classList.add("selected");

    if (destSet.has(name)) {
      sq.classList.add("dest");
      // capture if there is an enemy piece there (or en-passant target square is occupied conceptually).
      if (ch) sq.classList.add("capture");
    }

    // piece (SVG image)
    let pieceEl = sq.querySelector(".piece");
    if (ch) {
      const p = pieceFromChar(ch);
      const src = pieceSrc(p);
      if (!pieceEl) {
        pieceEl = el("img", "piece");
        pieceEl.draggable = false;
        pieceEl.alt = "";
        sq.appendChild(pieceEl);
      }
      if (pieceEl.getAttribute("src") !== src) pieceEl.setAttribute("src", src);
      pieceEl.className = "piece " + (p.color === "w" ? "white" : "black");
      pieceEl.dataset.color = p.color;
      pieceEl.dataset.type = p.type;
    } else if (pieceEl) {
      pieceEl.remove();
    }

    // movability: a square is grabbable/clickable if it's a human's own piece
    // whose side is to move, OR it is a legal destination of the current selection.
    const canStart =
      interactive && ch && pieceFromChar(ch).color === game.turn &&
      isHuman(playerOf(game.turn)) && game.dests[name] && game.dests[name].length;
    if (canStart || destSet.has(name)) sq.classList.add("movable");
  }
}

// Whether the board accepts human interaction right now.
function boardIsInteractive() {
  if (!game.inProgress) return false;
  if (game.thinking) return false;            // an engine is searching
  if (!isHuman(playerOf(game.turn))) return false; // not a human's turn
  return true;
}

function renderMoveList() {
  const list = $("#movelist");
  list.innerHTML = "";
  const moves = game.moves || [];
  // group into pairs by move number
  const rows = new Map(); // number -> { w, b }
  for (const m of moves) {
    if (!rows.has(m.number)) rows.set(m.number, { w: "", b: "" });
    rows.get(m.number)[m.color] = m.uci;
  }
  const lastIdx = moves.length - 1;
  const lastMove = lastIdx >= 0 ? moves[lastIdx] : null;

  for (const [num, pair] of rows) {
    const row = el("div", "move-row");
    const n = el("span", "move-num");
    n.textContent = num + ".";
    const w = el("span", "move-cell" + (pair.w ? "" : " empty"));
    w.textContent = pair.w || "·";
    const b = el("span", "move-cell" + (pair.b ? "" : " empty"));
    b.textContent = pair.b || "·";
    if (lastMove && lastMove.number === num && lastMove.color === "w") w.classList.add("current");
    if (lastMove && lastMove.number === num && lastMove.color === "b") b.classList.add("current");
    row.append(n, w, b);
    list.appendChild(row);
  }
  list.scrollTop = list.scrollHeight; // auto-scroll to bottom
}

// Choose which side's info to show: the one currently thinking, else last mover's,
// else white's. We always keep the last-seen info per side in ui.lastInfo.
function renderInfo() {
  let side = game.thinking;
  if (!side) {
    const moves = game.moves || [];
    side = moves.length ? moves[moves.length - 1].color : "w";
  }
  const info = ui.lastInfo[side];

  $("#i-depth").textContent = info && info.depth != null ? info.depth : "—";
  $("#i-score").textContent = info ? formatScore(info.score) : "—";
  $("#i-nodes").textContent = info && info.nodes != null ? humanize(info.nodes) : "—";
  $("#i-nps").textContent = info && info.nps != null ? humanize(info.nps) : "—";
  $("#i-time").textContent = info && info.timeMs != null ? formatTime(info.timeMs) : "—";
  $("#i-pv").textContent = info && info.pv && info.pv.length ? info.pv.join(" ") : "—";

  renderEvalBar(info ? info.score : null, side);
}

// Eval bar: white fills from bottom. Score is from `side`'s perspective; we
// normalise to White's perspective so the bar is consistent.
function renderEvalBar(score, side) {
  const fill = $("#evalbar-fill");
  const label = $("#evalbar-label");
  if (!score || typeof score.value !== "number") {
    fill.style.width = "50%";
    label.textContent = "0.00";
    return;
  }

  // Convert to White's point of view.
  const sign = side === "b" ? -1 : 1;

  if (score.type === "mate") {
    const m = score.value * sign; // normalise: positive => White is mating
    fill.style.width = m > 0 ? "100%" : "0%";
    label.textContent = (m < 0 ? "-M" : "M") + Math.abs(m);
    return;
  }

  const cpWhite = score.value * sign;
  const clamped = Math.max(-1000, Math.min(1000, cpWhite));
  const pct = (clamped + 1000) / 2000 * 100; // [-1000,1000] -> [0,100]
  fill.style.width = pct.toFixed(1) + "%";

  // Label is normalised to White's perspective so it agrees with the bar.
  const pawns = cpWhite / 100;
  label.textContent = (pawns >= 0 ? "+" : "") + pawns.toFixed(2);
}

function renderStatus() {
  const s = $("#status-line");
  s.classList.remove("thinking", "over");
  let text;

  if (!game.inProgress && game.result) {
    s.classList.add("over");
    text = statusForResult();
  } else if (!game.inProgress) {
    text = "No game in progress.";
  } else if (game.thinking) {
    s.classList.add("thinking");
    text = sideToWord(game.thinking) + " is thinking…";
  } else {
    text = sideToWord(game.turn) + " to move";
    if (game.check) text += " — check";
  }
  s.textContent = text;
}

function statusForResult() {
  const reason = game.reason;
  if (game.result === "1/2-1/2") {
    const r = reason ? "Draw — " + reason : "Draw";
    return r + " (½-½)";
  }
  const winner = game.result === "1-0" ? "White" : "Black";
  if (reason === "checkmate") return `Checkmate — ${winner} wins (${game.result})`;
  return `${winner} wins (${game.result})` + (reason ? ` — ${reason}` : "");
}

// Reflect player selects, mode toggle, FEN box, button enablement from state.
function renderControls() {
  // player dropdowns (only overwrite if value still valid / differs)
  syncSelect($("#sel-white"), game.players.white);
  syncSelect($("#sel-black"), game.players.black);
  renderEngineDescs();

  // FEN box (don't clobber while user is editing it)
  const fenBox = $("#fen-box");
  if (document.activeElement !== fenBox) fenBox.value = game.fen;

  // mode toggle
  document.querySelectorAll("#seg-mode .seg-btn").forEach((b) => {
    b.classList.toggle("active", b.dataset.mode === game.mode);
  });

  // Engine-vs-engine? (both sides are engines)
  const eve = !isHuman(game.players.white) && !isHuman(game.players.black);
  const thinking = !!game.thinking;

  // Step only meaningful in manual engine-vs-engine, when nobody is currently thinking.
  $("#btn-step").disabled = !(game.inProgress && eve && game.mode === "manual" && !thinking);
  // Move now: there is a side-to-move engine to nudge, game running, not already thinking.
  const sideEngine = game.inProgress && !isHuman(playerOf(game.turn));
  $("#btn-go").disabled = !(sideEngine && !thinking);
}

function syncSelect(sel, value) {
  // Only set if the option exists; otherwise leave the user's choice.
  if ([...sel.options].some((o) => o.value === value)) sel.value = value;
}

function renderConnection(connected) {
  $("#conn-banner").classList.toggle("hidden", connected);
}

// Full re-render from game state.
function renderAll() {
  renderBoard();
  renderMoveList();
  renderInfo();
  renderStatus();
  renderControls();
}

/* ===========================================================================
   4. BOARD INTERACTION
   =========================================================================== */

// --- click to select / move ---
boardEl.addEventListener("click", (ev) => {
  if (!boardIsInteractive()) return;
  if (ui.pendingPromo) return; // picker handles its own clicks
  const sqDiv = ev.target.closest(".square");
  if (!sqDiv) return;
  const name = sqDiv.dataset.sq;

  if (ui.selected) {
    if (name === ui.selected) {
      clearSelection();
      return;
    }
    const dests = game.dests[ui.selected] || [];
    if (dests.includes(name)) {
      attemptMove(ui.selected, name);
      return;
    }
    // clicked elsewhere: maybe re-select another own piece
    if (canSelect(name)) selectSquare(name);
    else clearSelection();
    return;
  }

  if (canSelect(name)) selectSquare(name);
});

function canSelect(name) {
  return !!(game.dests[name] && game.dests[name].length);
}

function selectSquare(name) {
  ui.selected = name;
  renderBoard();
}
function clearSelection() {
  if (ui.selected) {
    ui.selected = null;
    renderBoard();
  }
}

// --- drag and drop (pointer events; works for mouse + touch) ---
boardEl.addEventListener("pointerdown", (ev) => {
  if (!boardIsInteractive() || ui.pendingPromo) return;
  if (ev.button !== undefined && ev.button !== 0) return;
  const pieceEl = ev.target.closest(".piece");
  if (!pieceEl) return;
  const sqDiv = pieceEl.closest(".square");
  const from = sqDiv.dataset.sq;
  if (!canSelect(from)) return;

  ev.preventDefault();
  selectSquare(from); // shows dests immediately

  const ghost = el("img", "drag-ghost");
  ghost.src = pieceEl.getAttribute("src");
  ghost.draggable = false;
  const sz = sqDiv.getBoundingClientRect().width;
  ghost.style.width = sz + "px";
  ghost.style.height = sz + "px";
  document.body.appendChild(ghost);
  positionGhost(ghost, ev.clientX, ev.clientY);

  pieceEl.classList.add("dragging");

  ui.drag = { from, ghost, pieceEl, moved: false };

  const onMove = (e) => {
    ui.drag.moved = true;
    positionGhost(ghost, e.clientX, e.clientY);
    highlightHover(e.clientX, e.clientY);
  };
  const onUp = (e) => {
    window.removeEventListener("pointermove", onMove);
    window.removeEventListener("pointerup", onUp);
    finishDrag(e.clientX, e.clientY);
  };
  window.addEventListener("pointermove", onMove);
  window.addEventListener("pointerup", onUp);
});

function positionGhost(ghost, x, y) {
  ghost.style.left = x + "px";
  ghost.style.top = y + "px";
}

let hoverSquare = null;
function highlightHover(x, y) {
  const target = squareAt(x, y);
  if (target === hoverSquare) return;
  hoverSquare = target;
}

function squareAt(x, y) {
  const ely = document.elementFromPoint(x, y);
  const sqDiv = ely && ely.closest && ely.closest(".square");
  return sqDiv ? sqDiv.dataset.sq : null;
}

function finishDrag(x, y) {
  const drag = ui.drag;
  ui.drag = null;
  if (!drag) return;
  if (drag.ghost) drag.ghost.remove();
  if (drag.pieceEl) drag.pieceEl.classList.remove("dragging");

  const to = squareAt(x, y);
  if (!drag.moved) {
    // treated as a click-select; leave selection in place.
    return;
  }
  const dests = game.dests[drag.from] || [];
  if (to && dests.includes(to)) {
    attemptMove(drag.from, to);
  } else {
    clearSelection();
  }
}

// Decide whether a move needs a promotion picker, then either prompt or send.
function attemptMove(from, to) {
  const grid = parseFenBoard(game.fen);
  const ff = FILES.indexOf(from[0]), fr = RANKS.indexOf(from[1]);
  const ch = grid[fr] && grid[fr][ff];
  const isPawn = ch && ch.toLowerCase() === "p";
  const toRank = to[1];
  const promoting = isPawn && (toRank === "8" || toRank === "1");

  if (promoting) {
    openPromotion(from, to, game.turn);
  } else {
    sendMove(from, to, null);
  }
}

// --- promotion picker ---
function openPromotion(from, to, color) {
  ui.pendingPromo = { from, to, color };
  closePromotion(); // ensure clean

  const backdrop = el("div", "promo-backdrop");
  backdrop.addEventListener("click", () => { closePromotion(); clearSelection(); });

  const picker = el("div", "promo");
  picker.id = "promo-picker";
  const order = ["q", "r", "b", "n"];
  for (const t of order) {
    const cell = el("div", "promo-piece");
    const img = el("img", "promo-img");
    img.src = pieceSrc({ color, type: t });
    img.draggable = false;
    cell.appendChild(img);
    cell.addEventListener("click", (e) => {
      e.stopPropagation();
      sendMove(from, to, t);
      closePromotion();
    });
    picker.appendChild(cell);
  }

  // position over the destination square's column
  const destDiv = squareEls[to];
  const wrap = $("#board-wrap");
  const dRect = destDiv.getBoundingClientRect();
  const wRect = wrap.getBoundingClientRect();
  picker.style.width = dRect.width + "px";
  picker.style.left = (dRect.left - wRect.left) + "px";
  // Grow down if the dest is in the top half of the board, else up. This works
  // for either colour and for a flipped board, since it keys off visual position.
  const fromTop = (dRect.top - wRect.top);
  const destInTopHalf = (dRect.top - wRect.top) < wRect.height / 2;
  if (destInTopHalf) {
    picker.style.top = fromTop + "px";                       // grows downward
  } else {
    picker.style.top = (fromTop - dRect.height * 3) + "px";  // grows upward
  }

  wrap.appendChild(backdrop);
  wrap.appendChild(picker);
}

function closePromotion() {
  const p = $("#promo-picker");
  if (p) p.remove();
  const b = document.querySelector(".promo-backdrop");
  if (b) b.remove();
  ui.pendingPromo = null;
}

/* ===========================================================================
   5. NETWORKING
   =========================================================================== */

// --- outgoing actions ---
async function post(action, body) {
  try {
    const res = await fetch("/api/" + action, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body || {}),
    });
    let data = null;
    try { data = await res.json(); } catch (_) { /* empty body ok */ }
    if (!res.ok || (data && data.ok === false)) {
      const msg = (data && data.error) || `HTTP ${res.status}`;
      logLocal(`error: ${action} failed — ${msg}`);
    }
    return data;
  } catch (err) {
    logLocal(`error: ${action} request failed — ${err}`);
    return null;
  }
}

function sendMove(from, to, promotion) {
  clearSelection();
  const body = { from, to };
  if (promotion) body.promotion = promotion;
  post("move", body);
  // We do NOT optimistically update; wait for the next `state` event.
}

// --- incoming SSE ---
let evtSource = null;

function connect() {
  evtSource = new EventSource("/api/events");

  evtSource.onopen = () => renderConnection(true);

  evtSource.onmessage = (ev) => {
    let msg;
    try { msg = JSON.parse(ev.data); } catch (_) { return; }
    handleEvent(msg);
  };

  evtSource.onerror = () => {
    // EventSource auto-reconnects; just show the indicator.
    renderConnection(false);
  };
}

function handleEvent(msg) {
  if (!msg || typeof msg !== "object") return;
  switch (msg.type) {
    case "engines": onEngines(msg); break;
    case "state":   onState(msg); break;
    case "info":    onInfo(msg); break;
    case "uci":     onUci(msg); break;
    case "error":   onError(msg); break;
    default: break;
  }
}

function onEngines(msg) {
  ui.engines = Array.isArray(msg.list) ? msg.list : [];
  populatePlayerSelects();
}

let firstStateSeen = false;

function onState(msg) {
  const prevFen = game.fen;
  const prevCount = game.moves.length;

  // Replace game state wholesale; guard each field.
  game.fen = msg.fen || game.fen;
  game.turn = msg.turn === "b" ? "b" : "w";
  game.dests = msg.dests && typeof msg.dests === "object" ? msg.dests : {};
  game.lastMove = Array.isArray(msg.lastMove) ? msg.lastMove : null;
  game.check = !!msg.check;
  game.inProgress = !!msg.inProgress;
  game.result = msg.result || null;
  game.reason = msg.reason || null;
  game.moves = Array.isArray(msg.moves) ? msg.moves : [];
  game.players = msg.players && typeof msg.players === "object"
    ? { white: msg.players.white || "human", black: msg.players.black || "human" }
    : game.players;
  game.thinking = msg.thinking === "w" || msg.thinking === "b" ? msg.thinking : null;
  game.mode = msg.mode === "manual" ? "manual" : "auto";

  // selection no longer valid if it isn't a current source square
  if (ui.selected && !game.dests[ui.selected]) ui.selected = null;
  // a fresh state means any in-flight promotion is stale
  if (ui.pendingPromo) closePromotion();

  renderAll();

  // Play a sound for a newly-made move (but not for the initial snapshot).
  if (!firstStateSeen) firstStateSeen = true;
  else maybeSound(prevFen, prevCount);
}

// Decide which sound a just-played move warrants, by diffing the previous and
// new state, and play it. Only fires when exactly one new move appeared.
function maybeSound(prevFen, prevCount) {
  const count = game.moves.length;
  if (count !== prevCount + 1) return; // reset, FEN load, or a mid-game reconnect

  const lm = game.moves[count - 1];
  if (!lm || typeof lm.uci !== "string" || lm.uci.length < 4) return;

  const from = lm.uci.slice(0, 2), to = lm.uci.slice(2, 4);
  let kind = "move";

  if (countPieces(prevFen) > countPieces(game.fen)) kind = "capture"; // covers en passant too

  const grid = parseFenBoard(prevFen);
  const ff = FILES.indexOf(from[0]), fr = RANKS.indexOf(from[1]);
  const moved = grid[fr] && grid[fr][ff];
  if (moved && moved.toLowerCase() === "k" && Math.abs(FILES.indexOf(to[0]) - ff) === 2) kind = "castle";

  if (game.check) kind = "check";   // check/checkmate overrides
  if (!game.inProgress) kind = "end";

  Sound.play(kind);
}

function onInfo(msg) {
  const side = msg.color === "b" ? "b" : msg.color === "w" ? "w" : null;
  if (!side) return;
  // Merge so absent fields don't wipe previously-seen values within one search.
  const prev = ui.lastInfo[side] || {};
  ui.lastInfo[side] = {
    depth: msg.depth != null ? msg.depth : prev.depth,
    score: msg.score != null ? msg.score : prev.score,
    nodes: msg.nodes != null ? msg.nodes : prev.nodes,
    nps: msg.nps != null ? msg.nps : prev.nps,
    timeMs: msg.timeMs != null ? msg.timeMs : prev.timeMs,
    pv: Array.isArray(msg.pv) ? msg.pv : prev.pv,
  };
  renderInfo();
}

function onUci(msg) {
  appendUci(msg);
}

function onError(msg) {
  logLocal("error: " + (msg.message || "unknown error"));
}

/* ===========================================================================
   UCI CONSOLE
   =========================================================================== */

const MAX_LOG = 500;
const uciLogEl = $("#uci-log");

function appendUci(msg) {
  const engine = msg.engine || "?";
  const color = msg.color === "b" ? "b" : "w";
  const recv = msg.dir === "recv";
  const arrow = recv ? "«" : "»"; // « recv, » send
  const line = el("span", "uci-line " + (recv ? "recv" : "send"));

  const eng = el("span", "u-eng");
  eng.textContent = `${engine}(${color}) `;
  const arr = el("span", "u-arrow");
  arr.textContent = arrow + " ";
  const txt = el("span", "u-text");
  txt.textContent = msg.line != null ? msg.line : "";

  line.append(eng, arr, txt);
  pushLogLine(line);
}

function logLocal(text) {
  const line = el("span", "uci-line");
  const txt = el("span", "u-text");
  txt.textContent = text;
  line.appendChild(txt);
  pushLogLine(line);
}

function pushLogLine(node) {
  const atBottom =
    uciLogEl.scrollTop + uciLogEl.clientHeight >= uciLogEl.scrollHeight - 4;
  uciLogEl.appendChild(node);
  while (uciLogEl.childElementCount > MAX_LOG) {
    uciLogEl.removeChild(uciLogEl.firstChild);
  }
  if (atBottom) uciLogEl.scrollTop = uciLogEl.scrollHeight; // auto-scroll if pinned
}

/* ===========================================================================
   6. EVENT HANDLERS & WIRING
   =========================================================================== */

function populatePlayerSelects() {
  // Index the engine descriptors by id for quick name/description lookup.
  ui.engineById = {};
  for (const e of ui.engines) ui.engineById[e.id] = e;

  const mk = (sel, def) => {
    const prev = sel.value;
    sel.innerHTML = "";
    const optHuman = el("option");
    optHuman.value = "human";
    optHuman.textContent = "Human";
    sel.appendChild(optHuman);
    for (const e of ui.engines) {
      const o = el("option");
      o.value = e.id;
      o.textContent = e.name;
      sel.appendChild(o);
    }
    // restore previous choice if still valid, else apply default
    if ([...sel.options].some((o) => o.value === prev)) sel.value = prev;
    else sel.value = def;
  };

  // Defaults: White = Human, Black = the real engine if it's available, else the first one.
  const defaultBlack = (ui.engines.find((e) => e.id === "tryhard") || ui.engines[0] || { id: "human" }).id;
  mk($("#sel-white"), "human");
  mk($("#sel-black"), defaultBlack);

  renderEngineDescs();
  // reflect current server players if a game already exists
  renderControls();
}

// Presentation info for a player id ("human" or an engine id).
function engineInfo(id) {
  if (id === "human") return { name: "Human", description: "You play this side." };
  return ui.engineById[id] || { name: id, description: "" };
}

// Show the selected engine's description under each player selector.
function renderEngineDescs() {
  $("#desc-white").textContent = engineInfo($("#sel-white").value).description;
  $("#desc-black").textContent = engineInfo($("#sel-black").value).description;
}

function wire() {
  // New game
  $("#btn-new-game").addEventListener("click", () => {
    const fenRaw = $("#new-fen").value.trim();
    post("new_game", {
      white: $("#sel-white").value,
      black: $("#sel-black").value,
      fen: fenRaw ? fenRaw : null,
    });
  });

  // Update the descriptions when a player is changed.
  $("#sel-white").addEventListener("change", renderEngineDescs);
  $("#sel-black").addEventListener("change", renderEngineDescs);

  // Swap which engine plays which colour (applied on the next New game).
  $("#btn-swap").addEventListener("click", () => {
    const w = $("#sel-white").value;
    $("#sel-white").value = $("#sel-black").value;
    $("#sel-black").value = w;
    renderEngineDescs();
  });

  // Mode toggle (auto / manual)
  document.querySelectorAll("#seg-mode .seg-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      post("control", { mode: btn.dataset.mode });
    });
  });

  // Step / Move now
  $("#btn-step").addEventListener("click", () => post("step", {}));
  $("#btn-go").addEventListener("click", () => post("go", {}));

  // Search limits -> set_options on change.
  const sendOptions = () => {
    const mt = parseInt($("#opt-movetime").value, 10);
    const d = parseInt($("#opt-depth").value, 10);
    const body = {};
    body.movetime = isFinite(mt) && mt > 0 ? mt : 0;
    body.depth = isFinite(d) && d > 0 ? d : 0;
    post("set_options", body);
  };
  $("#opt-movetime").addEventListener("change", sendOptions);
  $("#opt-depth").addEventListener("change", sendOptions);

  // Board flip (frontend only — never hits the server)
  $("#btn-flip").addEventListener("click", () => {
    ui.flipped = !ui.flipped;
    closePromotion();
    buildBoard();   // rebuild square order
    renderBoard();
  });

  // Theme toggle (light / dark), persisted.
  $("#btn-theme").addEventListener("click", () => {
    applyTheme(currentTheme() === "dark" ? "light" : "dark");
  });

  // Sound toggle, persisted. Toggling also unlocks the audio context (a user gesture).
  $("#btn-sound").addEventListener("click", () => {
    Sound.set(!Sound.enabled());
    Sound.unlock();
    syncSoundButton();
  });

  // Browsers require a user gesture before audio can play; unlock on first interaction.
  window.addEventListener("pointerdown", () => Sound.unlock(), { once: true });

  // FEN set / copy
  $("#btn-set-fen").addEventListener("click", () => {
    const fen = $("#fen-box").value.trim();
    if (fen) post("set_fen", { fen });
  });
  $("#btn-copy-fen").addEventListener("click", async () => {
    const text = $("#fen-box").value;
    try {
      await navigator.clipboard.writeText(text);
    } catch (_) {
      // fallback for non-secure contexts
      const t = $("#fen-box");
      t.select();
      try { document.execCommand("copy"); } catch (__) {}
    }
  });

  // UCI console clear. It lives inside the panel's <summary>, so stop the click from toggling it.
  $("#console-clear").addEventListener("click", (e) => {
    e.preventDefault();
    e.stopPropagation();
    uciLogEl.innerHTML = "";
  });

  // Keep promotion picker / selection sane on resize.
  window.addEventListener("resize", () => {
    if (ui.pendingPromo) { closePromotion(); clearSelection(); }
  });
  // Escape clears selection / closes picker.
  window.addEventListener("keydown", (e) => {
    if (e.key === "Escape") { closePromotion(); clearSelection(); }
  });
}

// Remember which collapsible debugging panels are open across reloads.
function wireCollapsibles() {
  document.querySelectorAll("details.collapsible").forEach((d) => {
    const key = "sc-open-" + d.id;
    const saved = localStorage.getItem(key);
    if (saved !== null) d.open = saved === "1";
    d.addEventListener("toggle", () => localStorage.setItem(key, d.open ? "1" : "0"));
  });
}

// --- boot ---
function init() {
  applyTheme(currentTheme()); // sync the theme-toggle label to the pre-painted theme
  syncSoundButton();
  wireCollapsibles();
  buildBoard();
  populatePlayerSelects();
  wire();
  renderAll();
  connect();
}

document.addEventListener("DOMContentLoaded", init);
