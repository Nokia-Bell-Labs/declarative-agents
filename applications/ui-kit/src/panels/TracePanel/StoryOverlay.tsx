import { useMemo, useState } from "react";
import type { TraceModel, TraceSpan } from "../../api/traceApi";
import { useTraces } from "../../hooks/useTrace";
import { TraceOptionsProvider, useTraceReader, type TraceViewOptions } from "./options";
import { SpanDetail } from "./SpanDetail";
import { AgentGroupList, StepGroupList, type SpanSelection } from "./SpanRows";
import { agentGroups, continuations, stepGroups } from "./traceGroups";
import { CATEGORY_COLORS, timelineLanes } from "./traceLayout";
import { SpanTreeToggle } from "./Waterfall";
import { FULL_WINDOW, WindowBrush, type BrushWindow } from "./WindowBrush";

// The story window (GH-517): one trace window over an application's chats.
// Opened from a turn it shows that chat's turns; a checkbox menu overlays other
// chats (each trace normalized to its own start so shapes compare). With one
// chat visible the bars color by story category, with several by chat. The
// focused turn drives the Steps and Agents tabs. The application builds the
// chat list from its own history store and passes it in.

export interface TraceChatTurns {
  chatId: string;
  title: string;
  turns: Array<{ traceId: string; label: string }>;
}

export interface TraceStoryOverlayProps extends TraceViewOptions {
  chats: TraceChatTurns[];
  currentChatId?: string;
  focusTraceId?: string;
  onClose: () => void;
  // The agent ui.yaml names as the trace backend (srd004 R2.4).
  backend?: string;
  title?: string;
  emptyHint?: string;
}

const CHAT_COLORS = [
  "var(--chart-blue, #005aff)",
  "var(--chart-pink, #e03dcd)",
  "var(--chart-green, #37cc73)",
  "var(--chart-purple, #7d33f2)",
  "var(--chart-red, #e23b3b)",
  "var(--chart-teal, #23abb6)",
];

// Chats that carry a trace, with the launching chat pinned to the top.
function orderChats(chats: TraceChatTurns[], currentChatId?: string): TraceChatTurns[] {
  return chats.filter((chat) => chat.turns.length > 0).sort((a, b) => (a.chatId === currentChatId ? -1 : b.chatId === currentChatId ? 1 : 0));
}

interface Bar {
  span: TraceSpan;
  model: TraceModel;
  trace: string;
  external: boolean;
}

export function TraceStoryOverlay(props: TraceStoryOverlayProps) {
  return (
    <TraceOptionsProvider options={props}>
      <StoryWindow {...props} />
    </TraceOptionsProvider>
  );
}

function toggled<T>(set: Set<T>, value: T): Set<T> {
  const next = new Set(set);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}

function StoryWindow({
  chats,
  currentChatId,
  focusTraceId,
  onClose,
  backend = "collector",
  title = "Traces — the flow between entities",
  emptyHint = "No chat carries a trace yet — run a turn first.",
}: TraceStoryOverlayProps) {
  const index = useMemo(() => orderChats(chats, currentChatId), [chats, currentChatId]);
  // Opened without a launching turn, the first chat is selected and its last
  // turn takes the focus.
  const anchorChatId = currentChatId ?? index[0]?.chatId;
  const [selectedChats, setSelectedChats] = useState<Set<string>>(() => new Set(anchorChatId ? [anchorChatId] : []));
  const [pickerOpen, setPickerOpen] = useState(false);
  const [focus, setFocus] = useState(() => focusTraceId ?? index[0]?.turns[index[0].turns.length - 1]?.traceId ?? "");
  // Steps groups the window by the root machine's own commands, Agents by
  // emitting service; the chips and the brush scope either tab.
  const [tab, setTab] = useState<"steps" | "agents">("steps");
  const [selectedSpan, setSelectedSpan] = useState<string | undefined>(undefined);
  const [expandedAgents, setExpandedAgents] = useState<Set<string>>(new Set());
  const [range, setRange] = useState<BrushWindow>(FULL_WINDOW);

  const visible = index.filter((chat) => selectedChats.has(chat.chatId));
  const traceIds = visible.flatMap((chat) => chat.turns.map((turn) => turn.traceId));
  // A focus outside the listed turns (a judge run opened from its own table)
  // still fetches, so the overlay shows it.
  if (focus && !traceIds.includes(focus)) traceIds.push(focus);
  const models = useTraces(backend, traceIds);
  const chatOf = new Map<string, string>();
  for (const chat of visible) for (const turn of chat.turns) chatOf.set(turn.traceId, chat.chatId);
  const chatColor = (chatId: string) => CHAT_COLORS[Math.max(0, index.findIndex((c) => c.chatId === chatId)) % CHAT_COLORS.length];
  const chatName = (chat: TraceChatTurns) => (chat.chatId === currentChatId ? "this chat" : chat.title);
  const byChat = visible.length > 1;

  // The shared axis is each trace's own offset from its start; the window is
  // a fraction of the longest visible trace.
  const maxDur = Math.max(1, ...[...models.values()].map((m) => m.endUs - m.startUs));
  const w0 = range.start * maxDur;
  const w1 = range.end * maxDur;
  const focusModel = models.get(focus) ?? [...models.values()][0];
  const services = useMemo(() => new Set([...models.values()].flatMap((m) => m.services)), [models]);

  // Lanes: the union across visible traces of who was active in the window.
  const laneMap = new Map<string, Bar[]>();
  for (const [traceId, model] of models) {
    for (const lane of timelineLanes(model, model.startUs + w0, model.startUs + w1)) {
      const bars = laneMap.get(lane.name) ?? [];
      for (const span of lane.spans) bars.push({ span, model, trace: traceId, external: lane.external });
      laneMap.set(lane.name, bars);
    }
  }
  const earliest = (bars: Bar[]) => Math.min(...bars.map((x) => x.span.startUs - x.model.startUs));
  const lanes = [...laneMap.entries()].sort((a, b) => earliest(a[1]) - earliest(b[1]));

  const followTo = (span: TraceSpan) => {
    setSelectedSpan(span.id);
    setExpandedAgents((prev) => new Set(prev).add(span.service));
    requestAnimationFrame(() => document.getElementById(`span-row-${span.id}`)?.scrollIntoView?.({ block: "center" }));
  };
  const selection: SpanSelection = { selected: selectedSpan, onSelect: (id) => setSelectedSpan(id === selectedSpan ? undefined : id), onFollow: followTo };

  return (
    <div className="dak-trace">
      <div className="trace-overlay" role="dialog" aria-label="Trace story window" data-testid="trace-overlay">
        <div className="trace-overlay-head">
          <span>{title}</span>
          <div className="trace-chat-picker">
            <button type="button" className="detail-toggle" onClick={() => setPickerOpen((was) => !was)}>
              chats shown: {visible.length} ▾
            </button>
            {pickerOpen && (
              <div className="trace-chat-menu">
                {index.map((chat) => (
                  <label key={chat.chatId} className="trace-chat-option">
                    <input
                      type="checkbox"
                      checked={selectedChats.has(chat.chatId)}
                      onChange={(event) =>
                        setSelectedChats((current) => {
                          const next = new Set(current);
                          if (event.target.checked) next.add(chat.chatId);
                          else if (next.size > 1) next.delete(chat.chatId);
                          return next;
                        })
                      }
                    />
                    <span className="trace-chat-swatch" style={{ background: chatColor(chat.chatId) }} />
                    {chatName(chat)}
                    <span className="trace-chat-count">{chat.turns.length} turn(s)</span>
                  </label>
                ))}
              </div>
            )}
          </div>
          <button type="button" className="trace-overlay-close" onClick={onClose}>
            ✕ close
          </button>
        </div>
        <div className="trace-overlay-body">
          {index.length === 0 && <div className="trace-notice">{emptyHint}</div>}
          <div className="trace-turn-chips">
            {visible.flatMap((chat) =>
              chat.turns.map((turn) => (
                <button
                  type="button"
                  key={turn.traceId}
                  className={`turn-chip${turn.traceId === focus ? " turn-chip-active" : ""}`}
                  style={byChat ? { borderColor: chatColor(chat.chatId) } : undefined}
                  title={turn.label}
                  onClick={() => setFocus(turn.traceId)}
                >
                  {turn.label}
                </button>
              )),
            )}
          </div>
          {focusModel ? (
            <FocusedTurn
              focusModel={focusModel}
              services={services}
              brush={{ models, maxDur, range, setRange, activeLanes: lanes.length }}
              barColor={byChat ? (bar) => chatColor(chatOf.get(bar.trace) ?? "") : undefined}
              lanes={lanes}
              focus={focus}
              legend={byChat ? visible.map((chat) => [chatName(chat), chatColor(chat.chatId)] as [string, string]) : undefined}
              tab={tab}
              setTab={setTab}
              w={[w0, w1]}
              selection={selection}
              selectedSpan={selectedSpan}
              expanded={expandedAgents}
              onToggleAgent={(service) => setExpandedAgents((prev) => toggled(prev, service))}
              onCloseDetail={() => setSelectedSpan(undefined)}
            />
          ) : (
            index.length > 0 && <div className="trace-notice">The focused turn's spans have not arrived yet.</div>
          )}
        </div>
      </div>
    </div>
  );
}

// FocusedTurn is the overlay body once the focused trace has arrived: the
// brush, the lanes, the tabs, and the span drawer.
function FocusedTurn({
  focusModel,
  services,
  brush,
  barColor,
  lanes,
  focus,
  legend,
  tab,
  setTab,
  w: [w0, w1],
  selection,
  selectedSpan,
  expanded,
  onToggleAgent,
  onCloseDetail,
}: {
  focusModel: TraceModel;
  services: Set<string>;
  brush: { models: Map<string, TraceModel>; maxDur: number; range: BrushWindow; setRange: (next: BrushWindow) => void; activeLanes: number };
  barColor?: (bar: { span: TraceSpan; trace: string }) => string;
  lanes: Array<[string, Bar[]]>;
  focus: string;
  legend?: Array<[string, string]>;
  tab: "steps" | "agents";
  setTab: (tab: "steps" | "agents") => void;
  w: [number, number];
  selection: SpanSelection;
  selectedSpan: string | undefined;
  expanded: Set<string>;
  onToggleAgent: (service: string) => void;
  onCloseDetail: () => void;
}) {
  const reader = useTraceReader(focusModel, services);
  const startUs = focusModel.startUs + w0;
  const endUs = focusModel.startUs + w1;
  const colorOf = barColor ?? ((bar: { span: TraceSpan }) => CATEGORY_COLORS[reader.category(bar.span)]);
  const detail = focusModel.spans.find((span) => span.id === selectedSpan);
  const legendItems = legend ?? (Object.entries(CATEGORY_COLORS) as Array<[string, string]>);
  const tabButton = (id: "steps" | "agents", label: string) => (
    <button type="button" role="tab" aria-selected={tab === id} className={`trace-tab${tab === id ? " trace-tab-active" : ""}`} onClick={() => setTab(id)}>
      {label}
    </button>
  );
  return (
    <>
      <WindowBrush
        ticks={[...brush.models.entries()].flatMap(([traceId, model]) =>
          model.spans.map((s) => ({
            key: `${traceId}:${s.id}`,
            left: ((s.startUs - model.startUs) / brush.maxDur) * 100,
            width: (s.durationUs / brush.maxDur) * 100,
            color: colorOf({ span: s, trace: traceId }),
          })),
        )}
        range={brush.range}
        onChange={brush.setRange}
        windowUs={w1 - w0}
        activeLanes={brush.activeLanes}
      />
      <div className="trace-tabs" role="tablist">
        {tabButton("steps", "Steps")}
        {tabButton("agents", "Agents")}
      </div>
      {tab === "steps" && (
        <>
          {lanes.map(([name, bars]) => (
            <div className="timeline-lane" key={name}>
              <div className={`timeline-lane-name${bars.every((b) => b.external) ? " timeline-lane-external" : ""}`}>{name}</div>
              <div className="timeline-lane-track">
                {bars.map((bar) => {
                  const offset = bar.span.startUs - bar.model.startUs;
                  const left = ((Math.max(offset, w0) - w0) / (w1 - w0)) * 100;
                  const width = Math.max(0.4, ((Math.min(offset + bar.span.durationUs, w1) - Math.max(offset, w0)) / (w1 - w0)) * 100);
                  return (
                    <span
                      key={`${bar.trace}:${bar.span.id}`}
                      className={`timeline-bar${bar.trace === focus ? "" : " timeline-bar-ghost"}`}
                      style={{ left: `${left}%`, width: `${width}%`, background: colorOf(bar) }}
                      title={`${bar.span.command ?? bar.span.name} — ${(bar.span.durationUs / 1000).toFixed(1)} ms`}
                    />
                  );
                })}
              </div>
            </div>
          ))}
          <div className="timeline-legend">
            {legendItems.map(([name, color]) => (
              <span className="trace-legend-item" key={name}>
                <span className="trace-swatch" style={{ background: color }} />
                {name}
              </span>
            ))}
          </div>
          <div className="trace-head timeline-seq-head">The focused turn by workflow step — every agent's calls under the step that ran them; click a row for the span's content</div>
          <StepGroupList groups={stepGroups(focusModel, startUs, endUs)} trace={focusModel} reader={reader} selection={selection} />
        </>
      )}
      {tab === "agents" && (
        <>
          <div className="trace-head timeline-seq-head">The same window by agent — unfold an agent for the spans it emitted; follow a handoff to its continuation</div>
          <AgentGroupList groups={agentGroups(focusModel, startUs, endUs)} trace={focusModel} reader={reader} selection={selection} expanded={expanded} onToggle={onToggleAgent} />
          <SpanTreeToggle trace={focusModel} />
        </>
      )}
      {detail && <SpanDetail span={detail} model={focusModel} links={continuations(detail, focusModel.spans)} onFollow={selection.onFollow} onClose={onCloseDetail} />}
    </>
  );
}
