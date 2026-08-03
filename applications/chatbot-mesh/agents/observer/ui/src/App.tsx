import { AgentCard } from "./components/AgentCard";
import { StatusBar } from "./components/StatusBar";
import { Topology } from "./components/Topology";
import { metricsByPod } from "./fleet";
import { useFleet, type FleetSnapshot } from "./useFleet";

export function FleetView({ snapshot }: { snapshot: FleetSnapshot }) {
  const metrics = metricsByPod(snapshot.data.podMetrics);
  return (
    <main className="observer-shell">
      <header>
        <h1>Fleet Observer</h1>
        <p className="subtitle">Live mesh agent fleet view</p>
      </header>
      <StatusBar snapshot={snapshot} />
      <Topology data={snapshot.data} metrics={metrics} />
      {snapshot.data.agents.length ? (
        <>
          <h2 className="section-title">Agents</h2>
          <section className="grid" aria-label="Agents">
            {snapshot.data.agents.map((agent, index) => (
              <AgentCard
                agent={agent}
                resources={metrics[agent.name ?? ""]}
                key={`${agent.name ?? "unknown"}-${index}`}
              />
            ))}
          </section>
        </>
      ) : (
        <div className="empty">No agents discovered yet. Waiting for mesh pods...</div>
      )}
    </main>
  );
}

export default function App() {
  return <FleetView snapshot={useFleet()} />;
}
