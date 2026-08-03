// The artifact act of the showcase: a thing to use, not to read. It runs in
// agentbox's sandbox - no network, no storage, no way out except
// window.agentbox.emit(name, data), which is how a click in here reaches the agent
// that is waiting on it.
//
// The decision is the one a person actually has to make in a deploy: how much
// live traffic the new build takes before anybody trusts it. That is a number
// with consequences either side, which is exactly the kind of answer a sentence
// in a chat box is bad at and a slider is good at.
//
// The layout is three bands of fixed proportion on purpose: the showcase drives
// this with synthetic input, and percentages of the window are what let it aim.
import React, { useState } from "react";

const GREEN = "#22c55e";
const AMBER = "#f59e0b";

const RPM = 4200; // requests a minute the service is taking right now

function Bar({ percent }) {
  // The share of live traffic, as a bar rather than a number: the point of an
  // interface is that the consequence is visible before you commit to it.
  return (
    <div className="mt-4">
      <div className="flex h-8 overflow-hidden rounded-md border border-neutral-800">
        <div className="flex items-center justify-center text-xs font-bold text-black transition-all"
             style={{ width: `${percent}%`, background: GREEN }}>
          {percent >= 12 ? `new ${percent}%` : ""}
        </div>
        <div className="flex flex-1 items-center justify-center bg-neutral-800 text-xs text-neutral-400">
          {percent <= 88 ? `current build ${100 - percent}%` : ""}
        </div>
      </div>
      <div className="mt-2 flex justify-between text-xs text-neutral-500">
        <span>{Math.round((RPM * percent) / 100).toLocaleString()} req/min on the new build</span>
        <span>{RPM.toLocaleString()} req/min total</span>
      </div>
    </div>
  );
}

export default function Rollout() {
  const [percent, setPercent] = useState(10);
  const [sent, setSent] = useState(null);

  const decide = (action) => {
    setSent(action);
    window.agentbox.emit("rollout", { action, percent });
  };

  return (
    <div className="flex h-screen flex-col bg-neutral-950 p-6 font-sans text-neutral-200">
      {/* band 1: what is being decided */}
      <div className="flex-1">
        <div className="font-mono text-xs uppercase tracking-[0.3em]" style={{ color: GREEN }}>
          release 2026.7.3 · canary rollout
        </div>
        <div className="mt-3 text-2xl font-bold text-white">
          How much traffic should the new build take?
        </div>
        <p className="mt-2 max-w-xl text-sm leading-relaxed text-neutral-400">
          Tests are green and the migration is done. The rest is a judgement call: too little
          and the canary proves nothing, too much and a bad build reaches everybody.
        </p>
        <Bar percent={percent} />
      </div>

      {/* band 2: the number */}
      <div className="flex-1">
        <div className="flex items-baseline justify-between text-sm text-neutral-400">
          <span>share of live traffic</span>
          <span className="font-mono text-3xl font-bold tabular-nums" style={{ color: GREEN }}>
            {percent}%
          </span>
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
          className="mt-4 h-3 w-full cursor-pointer appearance-none rounded-full bg-neutral-800 accent-green-500"
        />
        <p className="mt-3 text-xs text-neutral-500">
          Every small move is reported, and repeated events of one name coalesce - so a dragged
          slider is one answer at the end, not forty interruptions on the way.
        </p>
      </div>

      {/* band 3: the two buttons the showcase clicks */}
      <div className="flex flex-1 items-end gap-4">
        <button
          onClick={() => decide("start")}
          className="h-24 flex-1 rounded-xl border-2 text-xl font-bold text-white transition"
          style={{ borderColor: GREEN, background: "rgba(34,197,94,0.14)" }}
        >
          Start the rollout
          <div className="mt-1 text-xs font-normal text-neutral-400">
            at {percent}%, and watch the error rate
          </div>
        </button>
        <button
          onClick={() => decide("hold")}
          className="h-24 flex-1 rounded-xl border-2 text-xl font-bold text-white transition"
          style={{ borderColor: AMBER, background: "rgba(245,158,11,0.14)" }}
        >
          Hold it
          <div className="mt-1 text-xs font-normal text-neutral-400">stay on the current build</div>
        </button>
      </div>

      {sent && (
        <div className="mt-4 text-center text-sm" style={{ color: GREEN }}>
          sent to the agent that was waiting: {sent} at {percent}%
        </div>
      )}
    </div>
  );
}
