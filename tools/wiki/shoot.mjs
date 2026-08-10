// Photograph one drawn frame, over the DevTools protocol.
//
// draw.py used to do this with `chrome --headless --screenshot
// --virtual-time-budget=8000`, and that flag is the whole reason this file
// exists. Virtual time fast-forwards the page's CLOCK, not its CPU: the budget
// is spent in a few real milliseconds, so anything that still needs actual work
// when it runs out is photographed unfinished. Every surface survived that. The
// artifact does not - it is a sandboxed iframe holding half a megabyte of inline
// React, and it came out an empty stage in one run and a working canary console
// in the next, from the same fixture. A wiki frame that is a coin toss is worse
// than a photograph.
//
// So: connect, navigate, and WAIT FOR THE PAGE TO SAY IT IS READY. runtime.js
// sets data-drawn once the surface has had its events and painted twice, and a
// frame can name a further selector that has to be there (an artifact's stage
// has to hold a frame with something in it). Then capture.
//
// Usage: node shoot.mjs <url> <width> <height> <out.png|-> [readySelector]
//
// An out of "-" measures instead of capturing: it prints the height the surface
// handed to bridge.fit(), which is the number Go sizes the window to.
//
// It speaks CDP directly because Node 22+ ships a WebSocket client, and adding
// puppeteer to the product's frontend so the docs can take a picture is a worse
// trade than sixty lines here.

import { writeFileSync } from "node:fs";

const [url, widthArg, heightArg, out, readySel] = process.argv.slice(2);
const width = Number(widthArg);
const height = Number(heightArg);
const SCALE = 2; // the frames are read on ordinary displays; 2x keeps text crisp
const READY_TIMEOUT_MS = 30_000;

const endpoint = process.env.AGENTBOX_CDP;
if (!endpoint) {
  console.error("shoot.mjs: AGENTBOX_CDP is unset (the browser's webSocketDebuggerUrl)");
  process.exit(2);
}

const ws = new WebSocket(endpoint);
let nextId = 1;
const pending = new Map();
const sessions = new Map();

function send(method, params = {}, sessionId) {
  const id = nextId++;
  ws.send(JSON.stringify({ id, method, params, sessionId }));
  return new Promise((resolve, reject) => pending.set(id, { resolve, reject }));
}

ws.addEventListener("message", (ev) => {
  const msg = JSON.parse(ev.data);
  if (msg.id && pending.has(msg.id)) {
    const { resolve, reject } = pending.get(msg.id);
    pending.delete(msg.id);
    msg.error ? reject(new Error(msg.error.message)) : resolve(msg.result);
  }
});

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function evaluate(session, expression) {
  const r = await send(
    "Runtime.evaluate",
    { expression, returnByValue: true, awaitPromise: true },
    session,
  );
  if (r.exceptionDetails) throw new Error(r.exceptionDetails.text);
  return r.result.value;
}

// The readiness question, asked of the page rather than assumed from a timer.
// An artifact's stage is the one that needs the extra clause: the iframe exists
// long before the program inside it has mounted, so an empty one is exactly the
// failure this is here to catch.
function readyExpr(selector) {
  const extra = selector ? JSON.stringify(selector) : "null";
  return `(() => {
    if (document.documentElement.dataset.drawn !== "1") return false;
    const sel = ${extra};
    if (!sel) return true;
    const el = document.querySelector(sel);
    if (!el) return false;
    const box = el.getBoundingClientRect();
    return box.width > 0 && box.height > 0;
  })()`;
}

async function main() {
  await new Promise((resolve, reject) => {
    ws.addEventListener("open", resolve, { once: true });
    ws.addEventListener("error", () => reject(new Error("cannot reach the browser")), { once: true });
  });

  const { targetId } = await send("Target.createTarget", { url: "about:blank" });
  const { sessionId } = await send("Target.attachToTarget", { targetId, flatten: true });
  sessions.set(targetId, sessionId);

  await send("Page.enable", {}, sessionId);
  await send("Runtime.enable", {}, sessionId);
  // deviceScaleFactor here rather than --force-device-scale-factor, so one
  // browser can take frames at any size without being restarted.
  await send(
    "Emulation.setDeviceMetricsOverride",
    { width, height, deviceScaleFactor: SCALE, mobile: false },
    sessionId,
  );

  await send("Page.navigate", { url }, sessionId);

  const deadline = Date.now() + READY_TIMEOUT_MS;
  let ready = false;
  while (Date.now() < deadline) {
    try {
      if (await evaluate(sessionId, readyExpr(readySel))) {
        ready = true;
        break;
      }
    } catch {
      // Navigation tears the context down under us; asking again is the answer.
    }
    await sleep(120);
  }
  if (!ready) {
    console.error(`shoot.mjs: ${url} never reported ready within ${READY_TIMEOUT_MS / 1000}s`);
    process.exit(3);
  }

  // Settle every animation before capturing. A card drops in over 120ms and the
  // strip's dot pulses forever; under the old virtual clock those were
  // fast-forwarded, and in real time they are whatever they happen to be when
  // the shutter opens - which showed up immediately as the same frame drawn
  // twice giving two different files. A drawn frame is the surface at rest, so
  // finish what ends and freeze what does not, in the page and in any
  // same-origin frame under it (the desk puts the surface in one).
  await evaluate(
    sessionId,
    `(() => {
      const settle = (win) => {
        for (const a of win.document.getAnimations()) {
          try {
            const t = a.effect?.getComputedTiming?.() ?? {};
            if (Number.isFinite(t.iterations)) {
              // Something that ends: show the end. A card's 120ms drop is a
              // transition into the state the frame is about.
              a.finish();
            } else {
              // Something that loops: freeze it somewhere legible. At zero the
              // progress window's indeterminate sweep sits off the left of its
              // own track, which reads as a bar that is not working.
              const d = Number(t.duration) || 1000;
              a.currentTime = d * 0.4;
            }
            a.pause();
          } catch {}
        }
      };
      settle(window);
      for (const f of document.querySelectorAll("iframe")) {
        try {
          if (f.contentWindow?.document) settle(f.contentWindow);
        } catch {}
      }
      return true;
    })()`,
  );

  // One more painted frame after ready, because a ResizeObserver-driven layout
  // settles a frame late and the surfaces measure themselves.
  await evaluate(sessionId, "new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)))");

  if (out === "-") {
    const h = await evaluate(sessionId, "document.documentElement.dataset.h || ''");
    process.stdout.write(String(h));
    await send("Target.closeTarget", { targetId });
    ws.close();
    return;
  }

  const shot = await send(
    "Page.captureScreenshot",
    { format: "png", captureBeyondViewport: false },
    sessionId,
  );
  writeFileSync(out, Buffer.from(shot.data, "base64"));
  await send("Target.closeTarget", { targetId });
  ws.close();
}

main().catch((err) => {
  console.error("shoot.mjs:", err.message);
  process.exit(1);
});
