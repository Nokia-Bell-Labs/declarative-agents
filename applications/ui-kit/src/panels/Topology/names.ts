import type { KubernetesObject } from "../../api/fleetApi";

// Short unit names (cohere-demo GH-369). The same agent answers to three
// names: its OTel service name in traces ("chatbot"), its Kubernetes object
// name in the inventory ("demo-chatbot-mesh-chatbot"), and its pod name on the
// cards ("demo-chatbot-mesh-chatbot-<hash>"). Every fleet surface leads with
// the first, derived by stripping the release prefix the deployment names
// share and any trailing replica-set and pod hashes.

// commonNamePrefix is the longest shared prefix of the deployment names,
// which is the release-and-chart prefix every mesh object carries.
export function commonNamePrefix(names: string[]): string {
  let prefix = names[0] ?? "";
  for (const name of names) {
    while (prefix && !name.startsWith(prefix)) prefix = prefix.slice(0, -1);
  }
  return prefix;
}

// shortUnitName strips the release prefix and, for pod names, the trailing
// generated segments: a Deployment pod carries a replica-set hash and a pod
// hash, a StatefulSet pod an ordinal. Segments that read as generated (hex-ish
// hashes or bare ordinals) are cut; real name segments ("fixture", "chroma")
// are kept.
export function shortUnitName(name: string, prefix: string): string {
  let short = name.startsWith(prefix) && name.length > prefix.length ? name.slice(prefix.length) : name;
  for (let cut = 0; cut < 2; cut++) {
    const at = short.lastIndexOf("-");
    if (at <= 0) break;
    const tail = short.slice(at + 1);
    // A pod's final segment may be all letters (wvjfq); it still reads as
    // generated when the segment before it is a digit-bearing replica-set
    // hash (cohere-demo GH-374).
    const before = short.slice(0, at).split("-").pop() ?? "";
    const generated =
      /^\d+$/.test(tail) ||
      (/^[a-z0-9]{5,10}$/.test(tail) && /\d/.test(tail)) ||
      (/^[a-z]{5}$/.test(tail) && /^[a-z0-9]{9,10}$/.test(before) && /\d/.test(before));
    if (!generated) break;
    short = short.slice(0, at);
  }
  return short;
}

// roleOf reads a workload's declared role from the annotation the chart puts
// beside it. The annotation key is the application's (cohere-demo uses
// mesh.cohere-demo/role), so the caller names it; without one there is no role.
export function roleOf(object: KubernetesObject, annotation?: string): string | undefined {
  if (!annotation) return undefined;
  return object.metadata?.annotations?.[annotation];
}
