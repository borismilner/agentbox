<!-- agentbox: title=How much traffic should the new build take? id=art-canary -->
import { useState } from "react";

// The canary console (wiki frame S7). An agent that needs a NUMBER rather than a
// choice writes the control for it and blocks until it comes back: nine numbered
// options cannot carry "somewhere between 5 and 100 percent", and a text box
// makes the person do arithmetic the agent already knows how to do.
//
// It emits once, with the share the human settled on, which is the whole answer.

const RPM_TOTAL = 4200;

export default function Canary() {
  const [share, setShare] = useState(50);
  const onNew = Math.round((RPM_TOTAL * share) / 100);

  const send = (verdict) =>
    window.agentbox?.emit("canary", { verdict, share, rpm: onNew });

  return (
    <div className="p-6 text-slate-100">
      <p className="text-xs uppercase tracking-widest text-slate-400">
        checkout-api · 2026.7.30 · eu-west
      </p>

      <div className="mt-6 flex items-baseline gap-3">
        <span className="text-5xl font-semibold tabular-nums">{share}%</span>
        <span className="text-sm text-slate-400">of live traffic</span>
      </div>

      <input
        type="range"
        min="0"
        max="100"
        step="5"
        value={share}
        onChange={(e) => setShare(Number(e.target.value))}
        className="mt-5 w-full accent-indigo-400"
      />

      <div className="mt-3 h-2 w-full overflow-hidden rounded bg-slate-700">
        <div className="h-full bg-indigo-400" style={{ width: `${share}%` }} />
      </div>

      <div className="mt-3 flex justify-between text-sm text-slate-300">
        <span className="tabular-nums">{onNew.toLocaleString()} req/min on the new build</span>
        <span className="tabular-nums text-slate-500">
          {(RPM_TOTAL - onNew).toLocaleString()} stay on 2026.7.22
        </span>
      </div>

      <div className="mt-6 flex gap-3">
        <button
          onClick={() => send("go")}
          className="rounded-lg bg-indigo-500 px-4 py-2 text-sm font-medium hover:bg-indigo-400"
        >
          Start the rollout
        </button>
        <button
          onClick={() => send("hold")}
          className="rounded-lg border border-slate-600 px-4 py-2 text-sm text-slate-300 hover:bg-slate-800"
        >
          Hold it
        </button>
      </div>
    </div>
  );
}
