import { useState } from "react";
import type { TraceModel } from "../../api/traceApi";
import { MachineDetail } from "./MachineDetail";
import { machineForWalk, orderedMachines } from "./machineViews";
import { useDeclared } from "./useDeclared";
import "./machineView.css";

// The agent panel of a trace view: the declared machine of an agent the trace
// ran through, with the states the turn was in filled, and its words as tags
// that open what they declare. A steps view reads the turn as what happened;
// this reads it as the program that made it happen. Base is agentic-wiki-mesh's
// AgentPanel. The application-specific joins it made are props: the program
// name each service runs (wiki resolved it from a generated surface) and the
// monitor agent each service is read through (cohere-demo's upstream map).

export interface AgentPanelProps {
  trace: TraceModel;
  // The program each trace service runs, shown beside the service name in the
  // picker when it differs. Default: none.
  serviceLabels?: ReadonlyMap<string, string>;
  // The monitored agent a trace service's declarations are read through.
  // Default: the service name itself.
  monitorAgentFor?: (service: string) => string;
  // Opening service. Default: the trace's first service.
  initialService?: string;
}

const NO_LABELS: ReadonlyMap<string, string> = new Map();
const identity = (service: string) => service;

export function AgentPanel({ trace, serviceLabels = NO_LABELS, monitorAgentFor = identity, initialService }: AgentPanelProps) {
  const [service, setService] = useState(initialService ?? trace.services[0] ?? "");
  const [machineName, setMachineName] = useState<string | undefined>(undefined);
  const declared = useDeclared(service === "" ? undefined : monitorAgentFor(service));

  const walk = trace.walks.find((candidate) => candidate.service === service);
  const ranIn = machineForWalk(declared.machines, walk);
  const ordered = orderedMachines(declared.machines, ranIn?.name);
  const selected = ordered.find((machine) => machine.name === machineName) ?? ordered[0];

  const chrome = (
    <div className="machine-bar">
      <span className="machine-head">Agent — the program behind these steps</span>
      <select
        className="agent-picker"
        data-testid="agent-picker"
        value={service}
        onChange={(event) => {
          setService(event.target.value);
          setMachineName(undefined);
        }}
      >
        {trace.services.map((name) => (
          <option key={name} value={name}>
            {name}
            {serviceLabels.get(name) && serviceLabels.get(name) !== name ? ` (${serviceLabels.get(name)})` : ""}
          </option>
        ))}
      </select>
      {ordered.length > 0 && (
        <select className="machine-picker" data-testid="machine-picker" value={selected?.name ?? ""} onChange={(event) => setMachineName(event.target.value)}>
          {ordered.map((machine) => (
            <option key={machine.name} value={machine.name}>
              {machine.name}
              {machine.name === ranIn?.name ? " — ran this turn" : ""}
            </option>
          ))}
        </select>
      )}
    </div>
  );

  if (declared.reading) {
    return (
      <div className="dak-machine agent-machine" data-testid="agent-machine">
        {chrome}
        <div className="machine-notice">Reading {service}'s declared machines…</div>
      </div>
    );
  }
  if (!selected) {
    return (
      <div className="dak-machine agent-machine" data-testid="agent-machine">
        {chrome}
        <div className="machine-notice machine-notice-warn" data-testid="machine-unavailable">
          {service} serves no declared machines through the monitor proxy. An agent exposes them from its monitor server as
          /monitor/machines; the rest of the view stays usable without it.
        </div>
      </div>
    );
  }

  return (
    <div className="dak-machine agent-machine" data-testid="agent-machine" data-service={service} data-machine={selected.name}>
      {chrome}
      <MachineDetail key={`${service}/${selected.name}`} machine={selected} tools={declared.tools} walk={walk} />
    </div>
  );
}

export default AgentPanel;
