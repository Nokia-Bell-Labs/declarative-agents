import { createContext, useContext, type ReactNode } from "react";
import { createKitClient, type KitClient } from "./client";

// Panels take their client from shell context (srd004 R6.2). Without a
// provider they read same-origin.
const KitClientContext = createContext<KitClient>(createKitClient());

export function KitClientProvider({ client, children }: { client: KitClient; children: ReactNode }) {
  return <KitClientContext.Provider value={client}>{children}</KitClientContext.Provider>;
}

export function useKitClient(): KitClient {
  return useContext(KitClientContext);
}
