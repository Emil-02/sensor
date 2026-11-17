// sensor web app. Everything runs locally: the Go detector is compiled to
// WebAssembly and OCR uses a self-hosted copy of tesseract.js.

const $ = (sel) => document.querySelector(sel);

// ---------- Go WASM detector ----------

const sensorReady = (async () => {
  const go = new Go();
  let result;
  try {
    result = await WebAssembly.instantiateStreaming(fetch('sensor.wasm'), go.importObject);
  } catch {
    const buf = await (await fetch('sensor.wasm')).arrayBuffer();
    result = await WebAssembly.instantiate(buf, go.importObject);
  }
  go.run(result.instance);
  for (let i = 0; i < 200 && !globalThis.sensor; i++) {
    await new Promise((r) => setTimeout(r, 10));
  }
  if (!globalThis.sensor) throw new Error('mesin deteksi gagal dimuat');
  return globalThis.sensor;
})();

async function scan(text) {
  const s = await sensorReady;
  return JSON.parse(s.scan(text, ''));
}

// ---------- Tabs ----------

for (const tab of document.querySelectorAll('[role="tab"]')) {
  tab.addEventListener('click', () => {
    for (const t of document.querySelectorAll('[role="tab"]')) {
      const on = t === tab;
      t.setAttribute('aria-selected', String(on));
      document.getElementById(t.getAttribute('aria-controls')).hidden = !on;
    }
  });
}

// ---------- Text tab ----------

const textIn = $('#text-in');
const textOut = $('#text-out');
const textMode = $('#text-mode');
let textSeq = 0;

async function updateText() {
  const seq = ++textSeq;
  const text = textIn.value;
  const findings = await scan(text);
  if (seq !== textSeq) return;
  textOut.replaceChildren();
  let last = 0;
  for (const f of findings) {
    textOut.append(text.slice(last, f.start));
    const mark = document.createElement('mark');
    mark.title = f.label;
    mark.textContent = textMode.value === 'label' ? `[${f.type.toUpperCase()}]` : f.masked;
    textOut.append(mark);
    last = f.end;
  }
  textOut.append(text.slice(last));
  $('#text-count').textContent = text ? `${findings.length} data pribadi disamarkan` : '';
}

textIn.addEventListener('input', updateText);
textMode.addEventListener('change', updateText);
$('#copy').addEventListener('click', async () => {
  await navigator.clipboard.writeText(textOut.textContent);
  $('#copy').textContent = 'Tersalin ✓';
  setTimeout(() => { $('#copy').textContent = 'Salin hasil'; }, 1500);
});

// ---------- Photo tab ----------

const canvas = $('#canvas');
const ctx = canvas.getContext('2d');
const MAX_SIDE = 2400;
const COVER = '#111111';

const state = {
  base: null, // canvas holding the original (downscaled) image
  auto: [], // [{label, masked, boxes: [{x,y,w,h}], on}]
  manual: [], // [{x,y,w,h}]
  drag: null,
};

let ocrWorker = null;
let onProgress = () => {};

const STATUS = {
  'loading tesseract core': 'Memuat mesin OCR…',
  'initializing tesseract': 'Menyiapkan OCR…',
  'initialized tesseract': 'Menyiapkan OCR…',
  'loading language traineddata': 'Memuat data bahasa Indonesia…',
  'loaded language traineddata': 'Memuat data bahasa Indonesia…',
  'initializing api': 'Menyiapkan OCR…',
  'initialized api': 'Menyiapkan OCR…',
  'recognizing text': 'Membaca teks di foto…',
};

function getWorker() {
  if (!ocrWorker) {
    const abs = (p) => new URL(p, location.href).href;
    // tesseract.js swallows language-loading errors and never settles the
    // createWorker promise, so failures are surfaced through errorHandler.
    let fail;
    const failed = new Promise((_, reject) => { fail = reject; });
    const created = Tesseract.createWorker('ind', 1, {
      workerPath: abs('vendor/tesseract/worker.min.js'),
      corePath: abs('vendor/core'),
      langPath: abs('lang'),
      workerBlobURL: false,
      gzip: true,
      logger: (m) => onProgress(m),
      errorHandler: (e) => fail(new Error(String(e))),
    });
    ocrWorker = Promise.race([created, failed]);
    ocrWorker.catch(() => { ocrWorker = null; });
  }
  return ocrWorker;
}

function setBusy(text, progress) {
  $('#busy').hidden = text == null;
  if (text != null) {
    $('#busy-text').textContent = text;
    const p = $('#busy-progress');
    if (progress == null) p.removeAttribute('value');
    else p.value = progress;
  }
}

async function imageToCanvas(source) {
  const bmp = await createImageBitmap(source, { imageOrientation: 'from-image' });
  const s = Math.min(1, MAX_SIDE / Math.max(bmp.width, bmp.height));
  const c = document.createElement('canvas');
  c.width = Math.round(bmp.width * s);
  c.height = Math.round(bmp.height * s);
  c.getContext('2d').drawImage(bmp, 0, 0, c.width, c.height);
  bmp.close();
  return c;
}

// OCR often confuses letters and digits inside numbers (O/0, I/1, S/5...).
// Replacements keep the string length so offsets stay valid.
const CONFUSABLE = { O: '0', o: '0', D: '0', Q: '0', I: '1', l: '1', i: '1', '|': '1', S: '5', s: '5', B: '8', Z: '2', z: '2', G: '6', b: '6', g: '9', q: '9' };
const isDigit = (c) => c >= '0' && c <= '9';

function fixDigits(word) {
  const digits = [...word].filter(isDigit).length;
  if (digits < 4) return word;
  let out = '';
  for (let i = 0; i < word.length; i++) {
    const c = word[i];
    const near = isDigit(word[i - 1] || '') || isDigit(word[i + 1] || '');
    out += CONFUSABLE[c] && near ? CONFUSABLE[c] : c;
  }
  return out;
}

async function ocr(base) {
  // Small photos are upscaled: Tesseract reads better at ~1600px width.
  const scale = Math.min(3, Math.max(1, 1600 / base.width));
  let src = base;
  if (scale > 1) {
    src = document.createElement('canvas');
    src.width = Math.round(base.width * scale);
    src.height = Math.round(base.height * scale);
    const c = src.getContext('2d');
    c.imageSmoothingQuality = 'high';
    c.drawImage(base, 0, 0, src.width, src.height);
  }
  const worker = await getWorker();
  const { data } = await worker.recognize(src, {}, { blocks: true, text: false });
  const lines = [];
  for (const block of data.blocks || []) {
    for (const para of block.paragraphs || []) {
      for (const line of para.lines || []) {
        const words = [];
        for (const w of line.words || []) {
          if (!w.text || !w.text.trim()) continue;
          words.push({
            text: w.text,
            bbox: { x0: w.bbox.x0 / scale, y0: w.bbox.y0 / scale, x1: w.bbox.x1 / scale, y1: w.bbox.y1 / scale },
          });
        }
        if (words.length) lines.push(words);
      }
    }
  }
  return visualRows(lines);
}

const median = (xs) => {
  const s = [...xs].sort((a, b) => a - b);
  return s.length ? s[Math.floor(s.length / 2)] : 0;
};

// Estimates how much the photo is tilted from the bottom edges of words
// Tesseract already put on the same line.
function skewSlope(lines) {
  const slopes = [];
  for (const ws of lines) {
    if (ws.length < 2) continue;
    const a = ws[0].bbox;
    const b = ws[ws.length - 1].bbox;
    const dx = (b.x0 + b.x1) / 2 - (a.x0 + a.x1) / 2;
    if (dx > 40) slopes.push((b.y1 - a.y1) / dx);
  }
  return Math.max(-0.35, Math.min(0.35, median(slopes)));
}

// Tesseract often puts the label column ("Nama") and the value column
// (": SITI AMINAH") of a KTP in different blocks. Regroup words into rows by
// their de-skewed vertical position so each label sits on the same line as
// its value, even when the photo is tilted.
function visualRows(lines) {
  const slope = skewSlope(lines);
  const words = lines.flat().map((w) => {
    const cx = (w.bbox.x0 + w.bbox.x1) / 2;
    return { ...w, y: (w.bbox.y0 + w.bbox.y1) / 2 - slope * cx, h: w.bbox.y1 - w.bbox.y0 };
  });
  const rows = [];
  for (const w of words.sort((a, b) => a.y - b.y)) {
    const row = rows.find((r) => Math.abs(w.y - r.y) < 0.5 * Math.max(w.h, r.h));
    if (row) {
      row.words.push(w);
      row.y = (row.y * (row.words.length - 1) + w.y) / row.words.length;
      row.h = Math.max(row.h, w.h);
    } else {
      rows.push({ y: w.y, h: w.h, words: [w] });
    }
  }
  return rows.sort((a, b) => a.y - b.y).map((r) => r.words.sort((a, b) => a.bbox.x0 - b.bbox.x0));
}

// Joins OCR words into text (one line per OCR line) and remembers where
// each word is, so findings can be mapped back to image rectangles.
function layout(lines) {
  let text = '';
  const spans = [];
  lines.forEach((words, li) => {
    if (li) text += '\n';
    words.forEach((w, wi) => {
      if (wi) text += ' ';
      const t = fixDigits(w.text);
      spans.push({ start: text.length, end: text.length + t.length, bbox: w.bbox, line: li });
      text += t;
    });
  });
  return { text, spans };
}

function boxesFor(f, spans) {
  const byLine = new Map();
  for (const s of spans) {
    if (s.start < f.end && s.end > f.start) {
      const b = byLine.get(s.line);
      byLine.set(s.line, b
        ? { x0: Math.min(b.x0, s.bbox.x0), y0: Math.min(b.y0, s.bbox.y0), x1: Math.max(b.x1, s.bbox.x1), y1: Math.max(b.y1, s.bbox.y1) }
        : { ...s.bbox });
    }
  }
  return [...byLine.values()].map((b) => {
    const pad = Math.max(3, (b.y1 - b.y0) * 0.2);
    return { x: b.x0 - pad, y: b.y0 - pad, w: b.x1 - b.x0 + 2 * pad, h: b.y1 - b.y0 + 2 * pad };
  });
}

function watermarkText() {
  const purpose = $('#wm-text').value.trim() || 'verifikasi';
  const date = new Date().toLocaleDateString('id-ID', { day: 'numeric', month: 'long', year: 'numeric' });
  $('#wm-preview').textContent = purpose;
  $('#wm-date').textContent = date;
  return `Hanya untuk ${purpose} · ${date}`;
}

function drawWatermark() {
  const text = watermarkText();
  if (!$('#wm-on').checked) return;
  const { width: W, height: H } = canvas;
  const size = Math.max(14, Math.round(Math.min(W, H) / 16));
  ctx.save();
  ctx.translate(W / 2, H / 2);
  ctx.rotate(-Math.PI / 7);
  ctx.font = `600 ${size}px system-ui, sans-serif`;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillStyle = 'rgba(220, 38, 38, 0.32)';
  ctx.strokeStyle = 'rgba(255, 255, 255, 0.25)';
  ctx.lineWidth = Math.max(1, size / 14);
  const stepY = size * 3;
  const stepX = ctx.measureText(text).width + size * 2;
  const R = Math.hypot(W, H);
  let row = 0;
  for (let y = -R; y <= R; y += stepY, row++) {
    for (let x = -R + (row % 2) * (stepX / 2); x <= R; x += stepX) {
      ctx.strokeText(text, x, y);
      ctx.fillText(text, x, y);
    }
  }
  ctx.restore();
}

function render({ forExport = false } = {}) {
  if (!state.base) return;
  ctx.drawImage(state.base, 0, 0);
  ctx.fillStyle = COVER;
  for (const item of state.auto) {
    if (item.on) for (const b of item.boxes) ctx.fillRect(b.x, b.y, b.w, b.h);
  }
  for (const b of state.manual) ctx.fillRect(b.x, b.y, b.w, b.h);
  drawWatermark();
  if (state.drag && !forExport) {
    const b = normRect(state.drag);
    ctx.fillStyle = 'rgba(17, 17, 17, 0.6)';
    ctx.fillRect(b.x, b.y, b.w, b.h);
    ctx.strokeStyle = '#0f766e';
    ctx.lineWidth = 2;
    ctx.strokeRect(b.x, b.y, b.w, b.h);
  }
}

function renderFound() {
  const ul = $('#found');
  ul.replaceChildren();
  if (!state.auto.length) {
    $('#found-hint').textContent = 'Tidak ada data yang terbaca otomatis. Tutup manual bagian yang sensitif.';
    return;
  }
  $('#found-hint').textContent = 'Centang untuk menutup.';
  state.auto.forEach((item) => {
    const li = document.createElement('li');
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.checked = item.on;
    cb.setAttribute('aria-label', item.label);
    cb.addEventListener('change', () => { item.on = cb.checked; render(); });
    const body = document.createElement('div');
    const kind = document.createElement('div');
    kind.className = 'kind';
    kind.textContent = item.label;
    const val = document.createElement('div');
    val.className = 'val';
    val.textContent = item.masked;
    body.append(kind, val);
    li.append(cb, body);
    ul.append(li);
  });
}

async function openImage(source) {
  let base;
  try {
    base = await imageToCanvas(source);
  } catch {
    alert('Gambar tidak bisa dibuka. Coba format JPG atau PNG.');
    return;
  }
  state.base = base;
  state.auto = [];
  state.manual = [];
  canvas.width = base.width;
  canvas.height = base.height;
  $('#drop').hidden = true;
  $('#editor').hidden = false;
  renderFound();
  $('#found-hint').textContent = 'Sedang membaca…';
  render();

  onProgress = (m) => setBusy(STATUS[m.status] || 'Memproses…', m.progress);
  setBusy('Memuat mesin OCR…', null);
  try {
    const { text, spans } = layout(await ocr(base));
    const findings = await scan(text);
    if (state.base !== base) return;
    state.auto = findings
      .map((f) => ({ label: f.label, masked: f.masked, boxes: boxesFor(f, spans), on: true }))
      .filter((it) => it.boxes.length);
  } catch (err) {
    console.error(err);
    if (state.base === base) alert('OCR gagal: ' + (err && err.message ? err.message : err) + '\nAnda tetap bisa menutup manual.');
  } finally {
    if (state.base === base) {
      setBusy(null);
      renderFound();
      render();
    }
  }
}

// Manual boxes

function toCanvas(e) {
  const r = canvas.getBoundingClientRect();
  return { x: ((e.clientX - r.left) * canvas.width) / r.width, y: ((e.clientY - r.top) * canvas.height) / r.height };
}

function normRect(d) {
  return { x: Math.min(d.x0, d.x1), y: Math.min(d.y0, d.y1), w: Math.abs(d.x1 - d.x0), h: Math.abs(d.y1 - d.y0) };
}

canvas.addEventListener('pointerdown', (e) => {
  if (!state.base) return;
  canvas.setPointerCapture(e.pointerId);
  const p = toCanvas(e);
  state.drag = { x0: p.x, y0: p.y, x1: p.x, y1: p.y };
});
canvas.addEventListener('pointermove', (e) => {
  if (!state.drag) return;
  const p = toCanvas(e);
  state.drag.x1 = p.x;
  state.drag.y1 = p.y;
  render();
});
const endDrag = () => {
  if (!state.drag) return;
  const b = normRect(state.drag);
  state.drag = null;
  if (b.w > 4 && b.h > 4) state.manual.push(b);
  render();
};
canvas.addEventListener('pointerup', endDrag);
canvas.addEventListener('pointercancel', endDrag);

$('#undo').addEventListener('click', () => { state.manual.pop(); render(); });
$('#clear-manual').addEventListener('click', () => { state.manual = []; render(); });
$('#wm-on').addEventListener('change', () => render());
$('#wm-text').addEventListener('input', () => render());

$('#download').addEventListener('click', () => {
  render({ forExport: true });
  canvas.toBlob((blob) => {
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `sensor-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.png`;
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }, 'image/png');
});

$('#reset').addEventListener('click', () => {
  state.base = null;
  state.auto = [];
  state.manual = [];
  setBusy(null);
  $('#editor').hidden = true;
  $('#drop').hidden = false;
  $('#file').value = '';
});

// Input: file picker, drag & drop, paste

$('#file').addEventListener('change', (e) => {
  const f = e.target.files[0];
  if (f) openImage(f);
});

const drop = $('#drop');
drop.addEventListener('dragover', (e) => { e.preventDefault(); drop.classList.add('over'); });
drop.addEventListener('dragleave', () => drop.classList.remove('over'));
drop.addEventListener('drop', (e) => {
  e.preventDefault();
  drop.classList.remove('over');
  const f = [...e.dataTransfer.files].find((x) => x.type.startsWith('image/'));
  if (f) openImage(f);
});
document.addEventListener('paste', (e) => {
  if ($('#panel-photo').hidden) return;
  const f = [...(e.clipboardData?.files || [])].find((x) => x.type.startsWith('image/'));
  if (f) {
    e.preventDefault();
    openImage(f);
  }
});

// Sample: a clearly fictitious KTP-like card for trying the tool.

function sampleKTP() {
  const c = document.createElement('canvas');
  c.width = 1280;
  c.height = 800;
  const g = c.getContext('2d');
  const grad = g.createLinearGradient(0, 0, 1280, 800);
  grad.addColorStop(0, '#dbeafe');
  grad.addColorStop(1, '#bfdbfe');
  g.fillStyle = grad;
  g.fillRect(0, 0, 1280, 800);
  g.fillStyle = '#0f172a';
  g.textAlign = 'center';
  g.font = 'bold 40px Arial, sans-serif';
  g.fillText('PROVINSI JAWA BARAT', 640, 70);
  g.fillText('KABUPATEN BOGOR', 640, 120);
  g.textAlign = 'left';
  const rows = [
    ['NIK', '3201014508900001'],
    ['Nama', 'SITI AMINAH'],
    ['Tempat/Tgl Lahir', 'BOGOR, 05-08-1990'],
    ['Jenis Kelamin', 'PEREMPUAN'],
    ['Alamat', 'JL. MAWAR NO. 12'],
    ['RT/RW', '002/005'],
    ['Kel/Desa', 'CIBINONG'],
    ['Kecamatan', 'CIBINONG'],
    ['Agama', 'ISLAM'],
    ['Pekerjaan', 'KARYAWAN SWASTA'],
  ];
  rows.forEach(([k, v], i) => {
    const y = 200 + i * 54;
    g.font = (i === 0 ? 'bold 38px' : '32px') + ' Arial, sans-serif';
    g.fillText(k, 50, y);
    g.fillText(': ' + v, 370, y);
  });
  g.fillStyle = '#94a3b8';
  g.fillRect(960, 190, 260, 330);
  g.fillStyle = '#334155';
  g.font = 'bold 28px Arial, sans-serif';
  g.textAlign = 'center';
  g.fillText('FOTO', 1090, 365);
  g.fillStyle = 'rgba(185, 28, 28, 0.85)';
  g.font = 'bold 30px Arial, sans-serif';
  g.fillText('CONTOH FIKTIF – BUKAN DATA ASLI', 640, 770);
  return new Promise((resolve) => c.toBlob(resolve, 'image/png'));
}

$('#sample').addEventListener('click', async () => openImage(await sampleKTP()));

watermarkText();
sensorReady.catch((err) => {
  console.error(err);
  alert('Gagal memuat mesin deteksi: ' + err.message);
});
