// The README's artifact screenshot: the decision a release actually needs -
// how much live traffic the canary takes. A number with consequences either
// side is what a slider is good at and a chat box is bad at, and the only way
// anything leaves this sandbox is window.agentbox.emit(name, data).
import React, { useState } from "react";

const GREEN = "#16a34a";
const REST = "#52525b";
const RPM = 4200;

function Tile({ label, value, unit, note }) {
  return (
    <div className="flex-1 rounded-lg border border-neutral-800 bg-neutral-900/60 px-4 py-3">
      <div className="text-xs text-neutral-500">{label}</div>
      <div className="mt-1 text-2xl font-semibold tabular-nums text-white">
        {value}
        {unit && <span className="ml-1 text-sm font-normal text-neutral-400">{unit}</span>}
      </div>
      <div className="mt-0.5 text-xs text-neutral-500">{note}</div>
    </div>
  );
}

function Split({ percent }) {
  return (
    <div className="mt-5">
      <div className="flex h-8 gap-[2px]">
        <div
          className="flex items-center justify-center rounded-md text-xs font-semibold text-white transition-all"
          style={{ width: `${Math.max(percent, 2)}%`, background: GREEN }}
        >
          {percent >= 8 ? `canary ${percent}%` : ""}
        </div>
        <div
          className="flex flex-1 items-center justify-center rounded-md text-xs font-medium text-neutral-200"
          style={{ background: REST }}
        >
          current build {100 - percent}%
        </div>
      </div>
      <div className="mt-2 flex justify-between text-xs text-neutral-500">
        <span>{Math.round((RPM * percent) / 100).toLocaleString()} req/min to the canary</span>
        <span>{RPM.toLocaleString()} req/min total</span>
      </div>
    </div>
  );
}

export default function Rollout() {
  const [percent, setPercent] = useState(10);

  return (
    <div className="flex h-screen flex-col bg-neutral-950 p-8 font-sans text-neutral-200">
      <div className="flex items-center justify-between">
        <span className="font-mono text-xs uppercase tracking-[0.3em]" style={{ color: "#22c55e" }}>
          checkout-api · release 2026.7.30
        </span>
        <span className="rounded-full border border-neutral-800 px-3 py-1 font-mono text-[11px] text-neutral-500">
          sandboxed · no network
        </span>
      </div>

      <h1 className="mt-4 text-2xl font-bold text-white">
        How much traffic should the canary take?
      </h1>
      <p className="mt-2 max-w-xl text-sm leading-relaxed text-neutral-400">
        Tests are green and the migration has run. Start small - you can raise
        the share at any time, and a bad build never reaches everybody at once.
      </p>

      <div className="mt-6 flex gap-3">
        <Tile label="error rate" value="0.04" unit="%" note="down from 0.06% an hour ago" />
        <Tile label="p95 latency" value="182" unit="ms" note="steady all afternoon" />
        <Tile label="live traffic" value="4,200" unit="req/min" note="typical for a weekday" />
      </div>

      <Split percent={percent} />

      <div className="mt-7">
        <div className="flex items-baseline justify-between">
          <span className="text-sm text-neutral-400">share of live traffic</span>
          <span className="text-3xl font-semibold tabular-nums text-white">{percent}%</span>
        </div>
        <input
          type="range"
          min="0"
          max="100"
          step="5"
          value={percent}
          onChange={(e) => {
            const v = Number(e.target.value);
            setPercent(v);
            window.agentbox.emit("percent", { value: v });
          }}
          className="mt-3 h-2.5 w-full cursor-pointer appearance-none rounded-full bg-neutral-800 accent-green-600"
        />
        <p className="mt-3 text-xs text-neutral-500">
          Drag freely. Repeated events of one name coalesce, so the agent gets
          one answer at the end - not forty on the way.
        </p>
      </div>

      <div className="mt-auto flex gap-4 pt-6">
        <button
          onClick={() => window.agentbox.emit("rollout", { action: "start", percent })}
          className="h-20 flex-1 rounded-xl border-2 text-lg font-bold text-white"
          style={{ borderColor: GREEN, background: "rgba(22,163,74,0.12)" }}
        >
          Start the rollout
          <div className="mt-1 text-xs font-normal text-neutral-400">
            at {percent}%, watching the error rate
          </div>
        </button>
        <button
          onClick={() => window.agentbox.emit("rollout", { action: "hold", percent })}
          className="h-20 flex-1 rounded-xl border-2 border-neutral-700 text-lg font-bold text-white"
          style={{ background: "rgba(82,82,91,0.12)" }}
        >
          Hold for now
          <div className="mt-1 text-xs font-normal text-neutral-400">stay on 2026.7.22</div>
        </button>
      </div>

      <div className="mt-4 text-center font-mono text-[11px] text-neutral-600">
        the one way out: window.agentbox.emit("rollout", {"{"}action, percent{"}"}) - the agent outside is blocked on it
      </div>
    </div>
  );
}
