"use strict";
const token = new URLSearchParams(location.search).get("t") || "";
const $ = id => document.getElementById(id);
const MAX_NAME = 27;

async function api(path, body) {
  const res = await fetch("/api/" + path, {
    method: body === undefined ? "GET" : "POST",
    headers: { "X-Token": token, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  let data = {};
  try { data = await res.json(); } catch (e) {}
  return { ok: res.ok, status: res.status, data };
}

// ---------------------------------------------------------------- the camera
let stream = null, timer = null, detector = null, lastText = "", lastAt = 0, busy = false;

async function startCamera(deviceId) {
  stopCamera();
  const video = { width: { ideal: 1920 }, height: { ideal: 1080 } };
  if (deviceId) video.deviceId = { exact: deviceId };
  try {
    stream = await navigator.mediaDevices.getUserMedia({ video });
  } catch (e) {
    setStatus("Cannot use the camera: " + (e.message || e.name) + ". Allow camera access for this page.", true);
    return;
  }
  $("video").srcObject = stream;
  const cams = (await navigator.mediaDevices.enumerateDevices()).filter(d => d.kind === "videoinput");
  const select = $("camera");
  const current = stream.getVideoTracks()[0].getSettings().deviceId;
  select.innerHTML = "";
  cams.forEach((cam, i) => {
    const option = document.createElement("option");
    option.value = cam.deviceId;
    option.textContent = cam.label || "Camera " + (i + 1);
    option.selected = cam.deviceId === current;
    select.appendChild(option);
  });
  $("cameraRow").hidden = cams.length < 2; // a choice is only worth showing if there is one
  if (!detector && "BarcodeDetector" in window) {
    try {
      if ((await BarcodeDetector.getSupportedFormats()).includes("qr_code")) detector = new BarcodeDetector({ formats: ["qr_code"] });
    } catch (e) {}
  }
  setStatus("Show the export QR code to the camera.");
  timer = setInterval(look, 200);
}

function stopCamera() {
  clearInterval(timer);
  timer = null;
  if (stream) stream.getTracks().forEach(t => t.stop());
  stream = null;
  $("video").srcObject = null;
}

const canvas = document.createElement("canvas");

async function look() {
  const video = $("video");
  if (busy || !stream || video.videoWidth === 0) return;
  busy = true;
  try {
    let text = "";
    if (detector) {
      const found = await detector.detect(video);
      if (found.length) text = found[0].rawValue;
    } else {
      canvas.width = video.videoWidth;
      canvas.height = video.videoHeight;
      const ctx = canvas.getContext("2d", { willReadFrequently: true });
      ctx.drawImage(video, 0, 0);
      const image = ctx.getImageData(0, 0, canvas.width, canvas.height);
      const code = jsQR(image.data, image.width, image.height, { inversionAttempts: "attemptBoth" });
      if (code) text = code.data;
    }
    const now = Date.now();
    if (text && (text !== lastText || now - lastAt > 3000)) {
      lastText = text;
      lastAt = now;
      await submit(text);
    }
  } catch (e) {
    // a frame that cannot be read is skipped
  } finally {
    busy = false;
  }
}

async function submit(text) {
  const res = await api("scan", { text });
  if (!res.ok) {
    setStatus((res.data.error || "Cannot read this code.").replace(/^./, c => c.toUpperCase()) + ".", true);
    return;
  }
  if (res.data.phase === "review") {
    showReview(res.data);
  } else {
    setStatus("Scanned code " + res.data.have + " of " + res.data.size + ": now show the next one.");
  }
}

function setStatus(text, bad) {
  $("status").textContent = text;
  $("status").classList.toggle("err", !!bad);
}

// ---------------------------------------------------------------- the review
let accounts = [], rows = [], polling = null;

function problem(name) {
  if (!name) return "empty";
  if (name.length > MAX_NAME) return name.length + "/" + MAX_NAME + " characters";
  if (/[^\x20-\x7e]/.test(name)) return "printable ASCII only";
  if (name[0] === " " || name[name.length - 1] === " ") return "starts or ends with a space";
  if (name.includes("  ")) return "two spaces in a row";
  return "";
}

function showReview(state) {
  stopCamera();
  $("scan").hidden = true;
  $("review").hidden = false;
  accounts = state.accounts;
  const box = $("rows");
  box.innerHTML = "";
  rows = accounts.map(account => {
    const row = document.createElement("div");
    row.className = "row" + (account.supported ? "" : " unsupported");
    const check = document.createElement("input");
    check.type = "checkbox";
    check.checked = account.supported && !state.done[account.id];
    check.disabled = !account.supported || !!state.done[account.id];
    const name = document.createElement("input");
    name.type = "text";
    name.value = account.name;
    name.disabled = check.disabled;
    name.spellcheck = false;
    const note = document.createElement("span");
    note.className = "note";
    note.textContent = account.supported ? "" : account.reason;
    row.append(check, name, note);
    box.appendChild(row);
    check.addEventListener("change", refresh);
    name.addEventListener("input", refresh);
    return { account, check, name, note, done: !!state.done[account.id] };
  });
  applyProgress(state);
  refresh();
}

function refresh() {
  const chosen = rows.filter(r => r.check.checked && !r.done && r.account.supported);
  const count = {};
  chosen.forEach(r => { const k = r.name.value.toLowerCase(); count[k] = (count[k] || 0) + 1; });
  let bad = 0;
  rows.forEach(r => {
    if (!r.account.supported || r.done) return;
    let msg = "";
    if (r.check.checked) {
      msg = problem(r.name.value);
      if (!msg && count[r.name.value.toLowerCase()] > 1) msg = "the same name twice";
    }
    if (msg) bad++;
    r.name.classList.toggle("bad", !!msg);
    if (!r.result) { r.note.textContent = msg; r.note.className = "note" + (msg ? " err" : ""); }
  });
  $("summary").textContent = chosen.length + " of " + rows.length + " selected" + (bad ? "; fix the red names" : "");
  $("import").disabled = !chosen.length || bad > 0 || polling !== null;
}

async function doImport() {
  const items = rows.filter(r => r.check.checked && !r.done && r.account.supported)
    .map(r => ({ id: r.account.id, name: r.name.value }));
  rows.forEach(r => { r.result = null; });
  const res = await api("import", { items });
  if (!res.ok) {
    const problems = res.data.problems || {};
    rows.forEach(r => {
      const msg = problems[r.account.id];
      if (msg) { r.note.textContent = msg; r.note.className = "note err"; r.name.classList.add("bad"); }
    });
    $("summary").textContent = res.data.error || "The import was refused.";
    return;
  }
  setBusy(true);
  polling = setInterval(poll, 500);
  poll();
}

function setBusy(on) {
  $("again").disabled = on;
  rows.forEach(r => {
    if (r.account.supported && !r.done) { r.check.disabled = on; r.name.disabled = on; }
  });
}

function applyProgress(state) {
  rows.forEach(r => {
    const st = state.rows[r.account.id];
    if (!st) return;
    r.result = st.state;
    r.note.textContent = st.text;
    r.note.className = "note " + (st.state === "ok" || st.state === "working" ? "ok" : "err");
    if (st.state === "ok" && !r.done) {
      r.done = true;
      r.check.checked = false;
      r.check.disabled = true;
      r.name.disabled = true;
      r.name.classList.remove("bad");
    }
  });
}

async function poll() {
  const res = await api("state");
  if (!res.ok) return;
  applyProgress(res.data);
  if (!res.data.running) {
    clearInterval(polling);
    polling = null;
    setBusy(false);
    const stored = rows.filter(r => r.done).length;
    const failed = rows.some(r => r.result === "denied" || r.result === "error");
    refresh();
    $("summary").textContent = stored + " key(s) stored on the device." + (failed ? " Fix what is shown and import again." : "");
  }
}

async function again() {
  const res = await api("reset", {});
  if (!res.ok) { $("summary").textContent = res.data.error || "Cannot start over."; return; }
  $("review").hidden = true;
  $("scan").hidden = false;
  accounts = []; rows = []; lastText = "";
  startCamera();
}

$("import").addEventListener("click", doImport);
$("again").addEventListener("click", again);
$("quit").addEventListener("click", async () => {
  stopCamera();
  await api("quit", {});
  document.body.innerHTML = "<p>Done. You can close this tab.</p>";
});
$("camera").addEventListener("change", e => startCamera(e.target.value));

(async () => {
  const res = await api("state");
  if (!res.ok) { setStatus("This page has no valid token: open the address the program printed.", true); return; }
  if (res.data.phase === "review") {
    showReview(res.data);
    if (res.data.running) { setBusy(true); polling = setInterval(poll, 500); }
  } else {
    startCamera();
  }
})();
