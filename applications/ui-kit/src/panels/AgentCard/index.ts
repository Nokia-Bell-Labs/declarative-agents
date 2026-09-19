import { definePanel } from "../manifest";
import { AgentCard, type AgentCardProps } from "./AgentCard";
import { AgentCards, AgentCardsMount, NO_AGENTS_TEXT, type AgentCardsProps } from "./AgentCards";
import { agentCardManifest } from "./manifest";

export { agentState, agentStateClass, isFailureSignal, orderAgents, shortTime, toolRecords, yamlify, type ToolRecord } from "./cardModel";
export { EventsSection, MachineSection, ResourcesLine, ToolsSection, WalkSection, type AgentMachine, type RenderMachine } from "./CardSections";
export { AgentCard, AgentCards, AgentCardsMount, agentCardManifest, NO_AGENTS_TEXT, type AgentCardProps, type AgentCardsProps };
export const agentCardPanel = definePanel(agentCardManifest, AgentCardsMount);
