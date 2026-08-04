import { objectName, type FleetData, type ResourceUsage } from "../fleet";

export function Topology({
  data,
  metrics,
}: {
  data: FleetData;
  metrics: Record<string, ResourceUsage>;
}) {
  if (!data.deployments.length && !data.services.length) return null;

  const serviceByComponent = new Map<string, string>();
  for (const service of data.services) {
    const component = service.spec?.selector?.["app.kubernetes.io/component"];
    if (component) serviceByComponent.set(component, objectName(service, "service"));
  }

  const podsByComponent = new Map<string, string[]>();
  for (const pod of data.pods) {
    const component = pod.metadata?.labels?.["app.kubernetes.io/component"];
    if (!component) continue;
    podsByComponent.set(component, [...(podsByComponent.get(component) ?? []), objectName(pod, "pod")]);
  }

  return (
    <section className="topology" aria-labelledby="topology-title">
      <h2 className="section-title" id="topology-title">Topology</h2>
      {data.deployments.map((deployment) => {
        const name = objectName(deployment, "deployment");
        const component = deployment.metadata?.labels?.["app.kubernetes.io/component"] ?? name;
        return (
          <div className="topo-group" key={name}>
            <h3>{name}</h3>
            {serviceByComponent.has(component) ? (
              <div className="topo-svc">service: {serviceByComponent.get(component)}</div>
            ) : null}
            <div className="topo-pods">
              {(podsByComponent.get(component) ?? []).map((pod) => {
                const usage = metrics[pod];
                const resource = usage ? `${usage.cpu ?? ""}/${usage.memory ?? ""}` : "n/a";
                return <span className="pod-chip" key={pod}>{pod} · {resource}</span>;
              })}
            </div>
          </div>
        );
      })}
    </section>
  );
}
