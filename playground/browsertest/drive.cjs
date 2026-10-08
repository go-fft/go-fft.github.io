// Copyright (c) the go-fft authors.
// SPDX-License-Identifier: BSD-3-Clause
//
// Browser proof of the go-fft playground: a real headless Chrome, synthetic
// mouse and keyboard input, and assertions on the canvas PIXELS and on the
// view-model the page exposes (gofftDebug). Run once per theme, each theme
// FORCED (Chrome otherwise inherits the host's appearance), and the page's own
// colours are checked first so a "light" run cannot silently be a dark one.
//
// usage: node drive.cjs <url> <outdir>   (CHROME=<path> to pick the browser)
// Screenshots go to <outdir>, which must not be inside a git work tree.
"use strict";
const fs = require("fs"), path = require("path");
const puppeteer = require("puppeteer-core");
const CHROME = process.env.CHROME || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const [url, out] = process.argv.slice(2);

// Captures must never land in a git work tree.
for (let d = path.resolve(out); ; d = path.dirname(d)) {
  if (fs.existsSync(path.join(d, ".git"))) { console.error("REFUSE: " + out + " is inside the git tree " + d); process.exit(2); }
  if (d === path.dirname(d)) break;
}
fs.mkdirSync(out, { recursive: true });

let failures = 0;
const ok = (cond, msg) => { console.log((cond ? "  PASS " : "  FAIL ") + msg); if (!cond) failures++; };

async function canvasPixel(page, cssX, cssY) {
  return page.evaluate((x, y) => {
    const c = document.getElementById("gofft-canvas");
    const s = c.width / c.clientWidth;
    const d = c.getContext("2d").getImageData(Math.round(x * s), Math.round(y * s), 1, 1).data;
    return [d[0], d[1], d[2]];
  }, cssX, cssY);
}
// count pixels within tolerance of rgb in a CSS rect of the canvas
async function countColor(page, r, rgb, tol = 40) {
  return page.evaluate((r, rgb, tol) => {
    const c = document.getElementById("gofft-canvas");
    const s = c.width / c.clientWidth;
    const d = c.getContext("2d").getImageData(Math.round(r[0] * s), Math.round(r[1] * s), Math.round(r[2] * s), Math.round(r[3] * s)).data;
    let n = 0;
    for (let i = 0; i < d.length; i += 4) if (Math.abs(d[i] - rgb[0]) + Math.abs(d[i + 1] - rgb[1]) + Math.abs(d[i + 2] - rgb[2]) <= tol) n++;
    return n;
  }, r, rgb, tol);
}
const hex = h => [1, 3, 5].map(i => parseInt(h.slice(i, i + 2), 16));
const dbg = page => page.evaluate(() => { const d = gofftDebug(); return JSON.parse(JSON.stringify(d)); });
const canvasOrigin = page => page.evaluate(() => { const r = document.getElementById("gofft-canvas").getBoundingClientRect(); return [r.left, r.top]; });
async function clickRect(page, r, fx = 0.5, fy = 0.5) {
  const [ox, oy] = await canvasOrigin(page);
  await page.mouse.click(ox + r[0] + r[2] * fx, oy + r[1] + r[3] * fy);
}
async function waitFor(page, fn, what, ms = 8000) {
  const t0 = Date.now();
  while (Date.now() - t0 < ms) { if (await page.evaluate(fn)) return true; await new Promise(r => setTimeout(r, 50)); }
  ok(false, "timed out waiting for " + what); return false;
}

async function run(theme) {
  console.log(`== theme forced: ${theme}`);
  const browser = await puppeteer.launch({
    executablePath: CHROME, headless: true,
    args: ["--no-sandbox", "--blink-settings=preferredColorScheme=" + (theme === "dark" ? 0 : 1), "--window-size=1440,900"],
    defaultViewport: { width: 1440, height: 900, deviceScaleFactor: 2 },
  });
  const ctx = browser.defaultBrowserContext();
  await ctx.overridePermissions(new URL(url).origin, ["clipboard-read", "clipboard-write", "clipboard-sanitized-write"]);
  const page = await browser.newPage();
  await page.emulateMediaFeatures([{ name: "prefers-color-scheme", value: theme }]);
  const errors = [];
  page.on("pageerror", e => errors.push(String(e)));
  page.on("console", m => { if (m.type() === "error") errors.push(m.text()); });
  await page.goto(url, { waitUntil: "load" });
  await waitFor(page, () => document.querySelector(".stage.ready") !== null, "the canvas to be revealed", 30000);
  await new Promise(r => setTimeout(r, 300));

  // The reference: does the PAGE look like the theme we forced?
  const bgCss = await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--bg").trim());
  const accentCss = await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--accent").trim());
  ok(bgCss === (theme === "dark" ? "#0b0e14" : "#ffffff"), `page --bg is the ${theme} one (${bgCss})`);
  // Pixels: the canvas background equals the page's --bg.
  const px = await canvasPixel(page, 4, 4);
  ok(JSON.stringify(px) === JSON.stringify(hex(bgCss)), `canvas background ${px} == page --bg ${hex(bgCss)}`);
  let d = await dbg(page);
  ok(d.problem === "", "no problem reported at start");
  ok(/RFFT/.test(d.outTitle) && d.outPoints === 501, `opening transform RFFT, 501 bins (${d.outTitle}, ${d.outPoints})`);
  ok(/round trip/.test(d.accuracy) && /per call/.test(d.speed), `accuracy and timing shown: ${d.accuracy} | ${d.speed}`);
  // Accent ink in the output plot: the spectrum is drawn in the brand colour.
  const acc = await countColor(page, d.rects.outPlot, hex(accentCss), 60);
  ok(acc > 200, `output plot carries ${acc} accent (${accentCss}) pixels`);
  // Text ink exists on the canvas (labels drawn).
  const inkCss = await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--ink").trim());
  ok(await countColor(page, [0, 0, 380, 300], hex(inkCss), 90) > 300, "control labels drawn in --ink");
  await page.screenshot({ path: path.join(out, `pg-${theme}-start.png`) });

  // 1. Pick FFT in the transform list WITH THE MOUSE: open it, click row 0.
  await clickRect(page, d.rects.transform);
  d = await dbg(page);
  ok(!!d.rects.popover, "clicking the transform picker opens its option list");
  if (d.rects.popover) {
    const p = d.rects.popover, rowH = p[3] / 8;
    await page.screenshot({ path: path.join(out, `pg-${theme}-popover.png`) });
    const [ox, oy] = await canvasOrigin(page);
    await page.mouse.click(ox + p[0] + p[2] / 2, oy + p[1] + rowH * 0.5);
  }
  d = await dbg(page);
  ok(d.transform === 0 && /^FFT/.test(d.outTitle) && d.outPoints === 1000, `FFT chosen: transform=${d.transform}, ${d.outTitle}, ${d.outPoints} points`);
  ok(/fft\.FFT\(xc\)/.test(d.goCode) && /np\.fft\.fft\(/.test(d.numpy), "Go and numpy code follow");

  // 2. Type a prime length into N: click the field, erase, type.
  await clickRect(page, d.rects.n);
  for (let i = 0; i < 6; i++) await page.keyboard.press("Backspace");
  await page.keyboard.type("1009");
  d = await dbg(page);
  ok(d.n === "1009" && /N = 1009 = prime/.test(d.length), `N typed: ${d.length}`);
  ok(d.outPoints === 1009, `1009 output bins (${d.outPoints})`);

  // 3. The view picker by KEYBOARD: focus it, open, ArrowDown, Enter → dB.
  await clickRect(page, d.rects.view);
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  d = await dbg(page);
  ok(/log10/.test(d.outTitle), `keyboard picks the dB view: ${d.outTitle}`);

  // 4. The fftshift switch.
  const before = d.goCode;
  await clickRect(page, d.rects.shift);
  d = await dbg(page);
  ok(/FFTShift/.test(d.goCode) && d.goCode !== before, "fftshift switch toggles the shift in the code");

  // 5. Copy Go → the system clipboard holds exactly the code shown.
  await clickRect(page, d.rects.copyGo);
  const clip = await page.evaluate(() => navigator.clipboard.readText());
  ok(clip === d.goCode, "Copy Go puts the Go code on the clipboard");

  // 6. Paste samples the way a visitor does: put text on the system
  // clipboard, click into the editor, press ⌘V. The canvas must leave the
  // shortcut to the browser, whose paste event carries the text in. Pasted
  // where the click put the caret: three more samples.
  await page.evaluate(() => navigator.clipboard.writeText(", 7, 8, 9"));
  await clickRect(page, d.rects.samples, 0.5, 0.2);
  await page.keyboard.down("Meta");
  await page.keyboard.press("KeyV", { commands: ["paste"] });
  await page.keyboard.up("Meta");
  await new Promise(r => setTimeout(r, 100));
  d = await dbg(page);
  ok(d.preset === 5 && d.n === "1012" && d.problem === "", `pasting 3 numbers into the editor switches to Custom with N=1012 (preset=${d.preset}, N=${d.n}, problem=${d.problem})`);

  // 7. Hover the input plot: the readout panel appears (pixels change).
  const ip = d.rects.inPlot;
  const shotBefore = await page.evaluate(r => { const c = document.getElementById("gofft-canvas"), s = c.width / c.clientWidth; return Array.from(c.getContext("2d").getImageData(r[0] * s, r[1] * s, r[2] * s, r[3] * s).data.slice(0, 400000)).join(","); }, ip);
  const [ox, oy] = await canvasOrigin(page);
  await page.mouse.move(ox + ip[0] + ip[2] * 0.4, oy + ip[1] + ip[3] * 0.5);
  await new Promise(r => setTimeout(r, 100));
  const shotAfter = await page.evaluate(r => { const c = document.getElementById("gofft-canvas"), s = c.width / c.clientWidth; return Array.from(c.getContext("2d").getImageData(r[0] * s, r[1] * s, r[2] * s, r[3] * s).data.slice(0, 400000)).join(","); }, ip);
  ok(shotBefore !== shotAfter, "hovering the input plot repaints it (readout)");
  await page.screenshot({ path: path.join(out, `pg-${theme}-after.png`) });

  // 8. Flip the theme with the page's own toggle: the canvas follows.
  const want = theme === "light" ? "#0b0e14" : "#ffffff";
  for (let i = 0; i < 3; i++) {
    await page.click("#theme-toggle");
    await new Promise(r => setTimeout(r, 150));
    const bg = await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--bg").trim());
    if (bg === want) break;
  }
  await new Promise(r => setTimeout(r, 200));
  const px2 = await canvasPixel(page, 4, 4);
  ok(JSON.stringify(px2) === JSON.stringify(hex(want)), `after the toggle the canvas background is ${px2}, want ${hex(want)}`);
  await page.screenshot({ path: path.join(out, `pg-${theme}-toggled.png`) });

  // 9. Phone width: the page itself never scrolls sideways; the stage does,
  // over a canvas that keeps its working size and still paints.
  await page.setViewport({ width: 390, height: 844, deviceScaleFactor: 2 }); // the ratio is read once, at start
  await new Promise(r => setTimeout(r, 400));
  const phone = await page.evaluate(() => {
    const st = document.querySelector(".stage"), c = document.getElementById("gofft-canvas");
    return { doc: document.documentElement.scrollWidth, stage: st.scrollWidth, cw: c.clientWidth, w: c.width };
  });
  ok(phone.doc <= 390 && phone.stage >= 1100 && phone.cw === 1100 && phone.w === 2200, `phone: page ${phone.doc}px wide, stage scrolls ${phone.stage}px, canvas ${phone.cw} CSS / ${phone.w} device px`);
  await page.screenshot({ path: path.join(out, `pg-${theme}-phone.png`) });

  ok(errors.length === 0, "no page errors" + (errors.length ? ": " + errors.join(" | ") : ""));
  await browser.close();
}

(async () => {
  for (const t of ["light", "dark"]) await run(t);
  console.log(failures ? `FAILURES: ${failures}` : "ALL PASS");
  process.exit(failures ? 1 : 0);
})();
