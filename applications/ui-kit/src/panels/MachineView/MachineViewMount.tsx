import { useState } from "react";
import type { MonitoredAgent } from "../../api/monitorApi";
import { useAgentMonitor } from "../../hooks/useAgentMonitor";
import type { PanelProps } from "../manifest";
import { MachineDetail } from "./MachineDetail";
import { orderedMachines } from "./machineViews";
import { useDeclared } from "./useDeclared";
import "./machineView.css";

// The mountable machine panel: pick one of the declared monitored agents and
// see its declared machines, with the live run's state and last transition
// drawn on the figure. An agent the proxy reports not deployed (srd004 R2.2)
// shows as not deployed, not as an error.

export interface MachineViewConfig {
  // The machine to open first, per agent. Default: the machine that holds the
  // agent's current state, else the first one the monitor serves.
  preferred_machines?: Record<string, string>;
  // Move the lifecycle supervisor (served first) to the end of the picker.
  // Default: false.
  supervisor_last?: boolean;
}

function LiveMachines({ agent, config }: { agent: MonitoredAgent; config: MachineViewConfig }) {
  const monitor = useAgentMonitor(agent.name);
  const declared = useDeclared(agent.name);
  const [machineName, setMachineName] = useState<string | undefined>(undefined);

  if (monitor.status === "absent") {
    return (
      <div className="machine-notice" data-testid="machine-not-deployed" data-hidden="true">
        {agent.label} is not deployed.
      </div>
    );
  }
  if (declared.reading || (declared.machines.length === 0 && monitor.status === "connecting")) {
    return <div className="machine-notice">Reading {agent.label}'s declared machines…</div>;
  }

  const runState = monitor.run?.state;
  const preferred =
    config.preferred_machines?.[agent.name] ?? declared.machines.find((machine) => runState !== undefined && machine.states.includes(runState))?.name;
  const ordered = orderedMachines(declared.machines, preferred, { supervisorLast: config.supervisor_last });
  const selected = ordered.find((machine) => machine.name === machineName) ?? ordered[0];
  if (!selected) {
    return (
      <div className="machine-notice machine-notice-warn" data-testid="machine-unavailable">
        {monitor.status === "error"
          ? `${agent.label}'s monitor is unreachable: ${monitor.lastError ?? "no answer"}.`
          : `${agent.label} serves no declared machines through the monitor proxy.`}
      </div>
    );
  }

  // The live state and transition are drawn only on the machine they belong to.
  const owns = (state?: string) => state !== undefined && selected.states.includes(state);
  const last = monitor.runEvents[0];
  const activeEdge = last && owns(last.fromState) && owns(last.toState) ? { from: last.fromState!, to: last.toState! } : undefined;

  return (
    <div className="machine-live" data-testid="machine-live" data-machine={selected.name}>
      {ordered.length > 1 && (
        <select className="machine-picker" data-testid="machine-picker" value={selected.name} onChange={(event) => setMachineName(event.target.value)}>
          {ordered.map((machine) => (
            <option key={machine.name} value={machine.name}>
              {machine.name}
              {runState !== undefined && machine.states.includes(runState) ? " — running" : ""}
            </option>
          ))}
        </select>
      )}
      <MachineDetail
        key={selected.name}
        machine={selected}
        tools={declared.tools}
        currentState={owns(runState) ? runState : undefined}
        activeEdge={activeEdge}
      />
    </div>
  );
}

// machineViewConfig reads the ui.yaml panel config, ignoring fields of the
// wrong shape rather than failing the panel.
export function machineViewConfig(config: Record<string, unknown> | undefined): MachineViewConfig {
  const out: MachineViewConfig = {};
  const preferred = config?.preferred_machines;
  if (preferred && typeof preferred === "object" && !Array.isArray(preferred)) {
    out.preferred_machines = Object.fromEntries(
      Object.entries(preferred).filter((entry): entry is [string, string] => typeof entry[1] === "string"),
    );
  }
  if (typeof config?.supervisor_last === "boolean") out.supervisor_last = config.supervisor_last;
  return out;
}

export function MachineViewMount({ monitoredAgents, config }: PanelProps) {
  const [agentName, setAgentName] = useState<string | undefined>(undefined);
  const agent = monitoredAgents.find((candidate) => candidate.name === agentName) ?? monitoredAgents[0];

  if (!agent) {
    return (
      <div className="dak-machine machine-panel" data-testid="machine-panel">
        <div className="machine-notice">No monitored agents are declared for this view.</div>
      </div>
    );
  }
  return (
    <div className="dak-machine machine-panel" data-testid="machine-panel" data-agent={agent.name}>
      <div className="machine-bar">
        <span className="machine-head">Machines</span>
        <select className="agent-picker" data-testid="agent-picker" value={agent.name} onChange={(event) => setAgentName(event.target.value)}>
          {monitoredAgents.map((candidate) => (
            <option key={candidate.name} value={candidate.name}>
              {candidate.label}
            </option>
          ))}
        </select>
      </div>
      <LiveMachines key={agent.name} agent={agent} config={machineViewConfig(config)} />
    </div>
  );
}
