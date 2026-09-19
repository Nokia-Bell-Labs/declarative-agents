import { objectName, type FleetData, type KubernetesObject, type ResourceUsage } from "../../api/fleetApi";
import { roleOf } from "./names";

// The service list the topology degrades to when no traces can be read. Base
// is the agentic-wiki-mesh and chatbot-mesh Topology: one group per
// deployment with its Service and pods. A unit is matched by its component
// label, read from metadata first (wiki-mesh GH-105: selectors may carry a
// frozen workload key and no role label) and from the selector second (the
// chatbot-mesh spelling), then by the deployment's selector against pod and
// Service labels, so a chart that sets no component label still groups.

const COMPONENT_LABEL = "app.kubernetes.io/component";

function componentOf(object: KubernetesObject): string | undefined {
  return object.metadata?.labels?.[COMPONENT_LABEL] ?? object.spec?.selector?.[COMPONENT_LABEL];
}

function selectorMatches(selector: Record<string, string> | undefined, labels: Record<string, string> | undefined): boolean {
  const entries = Object.entries(selector ?? {});
  return entries.length > 0 && entries.every(([key, value]) => labels?.[key] === value);
}

export interface TopologyServiceGroup {
  deployment: string;
  role?: string;
  services: string[];
  pods: string[];
}

// serviceGroups groups the fleet inventory by deployment.
export function serviceGroups(data: FleetData, roleAnnotation?: string): TopologyServiceGroup[] {
  return data.deployments.map((deployment) => {
    const name = objectName(deployment, "deployment");
    const component = componentOf(deployment) ?? name;
    const selector = deployment.spec?.selector;
    const belongs = (object: KubernetesObject, labels?: Record<string, string>) => componentOf(object) === component || selectorMatches(selector, labels);
    return {
      deployment: name,
      role: roleOf(deployment, roleAnnotation),
      services: data.services.filter((service) => belongs(service, service.spec?.selector)).map((service) => objectName(service, "service")),
      pods: data.pods.filter((pod) => belongs(pod, pod.metadata?.labels)).map((pod) => objectName(pod, "pod")),
    };
  });
}

export function ServiceList({ data, metrics, roleAnnotation }: { data: FleetData; metrics: Record<string, ResourceUsage>; roleAnnotation?: string }) {
  const groups = serviceGroups(data, roleAnnotation);
  if (groups.length === 0 && data.services.length === 0) {
    return <div className="topo-graph-empty">no deployments or services discovered yet</div>;
  }
  return (
    <div className="topo-list" data-testid="topology-list">
      {groups.map((group) => (
        <div className="topo-group" key={group.deployment}>
          <h3>{group.deployment}</h3>
          {group.role ? <div className="topo-role">{group.role}</div> : null}
          {group.services.map((service) => (
            <div className="topo-svc" key={service}>
              service: {service}
            </div>
          ))}
          <div className="topo-pods">
            {group.pods.map((pod) => {
              const usage = metrics[pod];
              const resource = usage ? `${usage.cpu ?? ""}/${usage.memory ?? ""}` : "n/a";
              return (
                <span className="pod-chip" key={pod}>
                  {pod} · {resource}
                </span>
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
}
