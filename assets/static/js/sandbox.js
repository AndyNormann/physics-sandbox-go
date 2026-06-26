// sandbox.js — the canvas client.
//
//   * Opens a native EventSource on /stream and parses Datastar's
//     `datastar-patch-signals` events itself, decoding the base64 binary frame
//     into typed arrays.
//   * Draws particles and remote cursors to <canvas> on a requestAnimationFrame
//     loop (decoupled from the network rate).
//   * Captures pointer drags, throttles them, and POSTs world-space cursor
//     state to /input.

const canvas = document.getElementById('scene');
const ctx = canvas.getContext('2d', { alpha: false });

// Latest decoded frame, swapped in by the SSE handler and read by rAF.
let frame = {
  worldW: 1920,
  worldH: 1080,
  px: new Int16Array(0), // interleaved x,y
  cursors: [],           // {x,y,r,g,b}
};

// ---- canvas sizing ------------------------------------------------------

let dpr = 1;
function resize() {
  dpr = window.devicePixelRatio || 1;
  canvas.width = Math.floor(canvas.clientWidth * dpr);
  canvas.height = Math.floor(canvas.clientHeight * dpr);
}
window.addEventListener('resize', resize);
resize();

// World->screen scaling preserving aspect (letterboxed).
function transform() {
  const sx = canvas.width / frame.worldW;
  const sy = canvas.height / frame.worldH;
  const s = Math.min(sx, sy);
  const ox = (canvas.width - frame.worldW * s) / 2;
  const oy = (canvas.height - frame.worldH * s) / 2;
  return { s, ox, oy };
}

// ---- SSE frame decoding -------------------------------------------------

function decode(b64) {
  const bin = atob(b64);
  const len = bin.length;
  const bytes = new Uint8Array(len);
  for (let i = 0; i < len; i++) bytes[i] = bin.charCodeAt(i);
  const dv = new DataView(bytes.buffer);

  const n = dv.getUint16(0, true);
  const c = dv.getUint16(2, true);
  const worldW = dv.getUint16(4, true);
  const worldH = dv.getUint16(6, true);

  let o = 8;
  const px = new Int16Array(n * 2);
  for (let i = 0; i < n; i++) {
    px[i * 2] = dv.getInt16(o, true);
    px[i * 2 + 1] = dv.getInt16(o + 2, true);
    o += 4;
  }
  const cursors = [];
  for (let i = 0; i < c; i++) {
    cursors.push({
      x: dv.getInt16(o, true),
      y: dv.getInt16(o + 2, true),
      r: bytes[o + 4],
      g: bytes[o + 5],
      b: bytes[o + 6],
    });
    o += 7;
  }
  frame = { worldW, worldH, px, cursors };
}

const es = new EventSource('/stream');
es.addEventListener('datastar-patch-signals', (e) => {
  // data line is `signals {json}`
  const idx = e.data.indexOf('{');
  if (idx < 0) return;
  let payload;
  try {
    payload = JSON.parse(e.data.slice(idx));
  } catch {
    return;
  }
  if (payload.frame) decode(payload.frame);
  if (payload.stats) updateStats(payload.stats);
});

// ---- stats overlay ------------------------------------------------------

const statEls = {
  particles: document.getElementById('stat-particles'),
  clients: document.getElementById('stat-clients'),
  tick: document.getElementById('stat-tick'),
  frame: document.getElementById('stat-frame'),
  goroutines: document.getElementById('stat-goroutines'),
  fps: document.getElementById('stat-fps'),
};
function updateStats(s) {
  statEls.particles.textContent = s.particles;
  statEls.clients.textContent = s.clients;
  statEls.tick.textContent = s.tickMs.toFixed(2);
  statEls.frame.textContent = s.frameBytes;
  statEls.goroutines.textContent = s.goroutines;
}

// ---- render loop --------------------------------------------------------

let frames = 0;
let lastFpsT = performance.now();
function render() {
  ctx.fillStyle = '#09090b';
  ctx.fillRect(0, 0, canvas.width, canvas.height);

  const { s, ox, oy } = transform();
  const px = frame.px;

  // Particles as additive glowing dots.
  ctx.globalCompositeOperation = 'lighter';
  ctx.fillStyle = 'rgba(110,231,183,0.85)';
  const rad = Math.max(1, 1.2 * dpr);
  for (let i = 0; i < px.length; i += 2) {
    const x = ox + px[i] * s;
    const y = oy + px[i + 1] * s;
    ctx.fillRect(x - rad / 2, y - rad / 2, rad, rad);
  }
  ctx.globalCompositeOperation = 'source-over';

  // Remote cursors.
  for (const cu of frame.cursors) {
    const x = ox + cu.x * s;
    const y = oy + cu.y * s;
    ctx.beginPath();
    ctx.arc(x, y, 8 * dpr, 0, Math.PI * 2);
    ctx.strokeStyle = `rgb(${cu.r},${cu.g},${cu.b})`;
    ctx.lineWidth = 2 * dpr;
    ctx.stroke();
  }

  frames++;
  const now = performance.now();
  if (now - lastFpsT >= 500) {
    statEls.fps.textContent = Math.round((frames * 1000) / (now - lastFpsT));
    frames = 0;
    lastFpsT = now;
  }
  requestAnimationFrame(render);
}
requestAnimationFrame(render);

// ---- input --------------------------------------------------------------

// Convert a screen (client) coordinate to world units.
function toWorld(clientX, clientY) {
  const rect = canvas.getBoundingClientRect();
  const cx = (clientX - rect.left) * dpr;
  const cy = (clientY - rect.top) * dpr;
  const { s, ox, oy } = transform();
  return { x: (cx - ox) / s, y: (cy - oy) / s };
}

let dragging = false;
let lastX = 0, lastY = 0, lastT = 0;
let pending = null;

canvas.addEventListener('pointerdown', (e) => {
  dragging = true;
  const w = toWorld(e.clientX, e.clientY);
  lastX = w.x; lastY = w.y; lastT = performance.now();
  canvas.setPointerCapture(e.pointerId);
});
canvas.addEventListener('pointermove', (e) => {
  if (!dragging) return;
  const w = toWorld(e.clientX, e.clientY);
  const now = performance.now();
  const dt = Math.max(0.001, (now - lastT) / 1000);
  pending = {
    x: w.x, y: w.y,
    dvx: (w.x - lastX) / dt,
    dvy: (w.y - lastY) / dt,
    active: true,
  };
  lastX = w.x; lastY = w.y; lastT = now;
});
function endDrag(e) {
  if (!dragging) return;
  dragging = false;
  send({ x: lastX, y: lastY, dvx: 0, dvy: 0, active: false });
  pending = null;
}
canvas.addEventListener('pointerup', endDrag);
canvas.addEventListener('pointercancel', endDrag);

function send(msg) {
  fetch('/input', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(msg),
    keepalive: true,
  }).catch(() => {});
}

// Throttle drag updates to ~50Hz.
setInterval(() => {
  if (pending) { send(pending); pending = null; }
}, 20);

// ---- particle count slider ----------------------------------------------

const count = document.getElementById('count');
const countLabel = document.getElementById('count-label');
let countTimer = null;
count.addEventListener('input', () => {
  countLabel.textContent = count.value;
  clearTimeout(countTimer);
  countTimer = setTimeout(() => {
    fetch('/count?n=' + count.value, { method: 'POST' }).catch(() => {});
  }, 150);
});
