// The double-ended brush over a trace overview (GH-511): ticks for every span,
// the selected window shaded, two range handles, and the window's size. The
// window is a pair of fractions of the whole [0..1]; the caller owns it.

export interface BrushTick {
  key: string;
  left: number;
  width: number;
  color: string;
}

export interface BrushWindow {
  start: number;
  end: number;
}

export const FULL_WINDOW: BrushWindow = { start: 0, end: 1 };

export function WindowBrush({
  ticks,
  range,
  onChange,
  windowUs,
  activeLanes,
}: {
  ticks: BrushTick[];
  range: BrushWindow;
  onChange: (next: BrushWindow) => void;
  windowUs: number;
  activeLanes: number;
}) {
  const zoomed = range.start > 0 || range.end < 1;
  return (
    <div className="timeline-brush">
      <div className="timeline-overview">
        {ticks.map((tick) => (
          <span key={tick.key} className="timeline-tick" style={{ left: `${tick.left}%`, width: `${Math.max(0.3, tick.width)}%`, background: tick.color }} />
        ))}
        <div className="timeline-window" style={{ left: `${range.start * 100}%`, width: `${(range.end - range.start) * 100}%` }} />
      </div>
      <input
        type="range"
        className="timeline-handle timeline-handle-start"
        min={0}
        max={1000}
        value={Math.round(range.start * 1000)}
        aria-label="window start"
        onChange={(event) => onChange({ ...range, start: Math.min(Number(event.target.value) / 1000, range.end - 0.01) })}
      />
      <input
        type="range"
        className="timeline-handle timeline-handle-end"
        min={0}
        max={1000}
        value={Math.round(range.end * 1000)}
        aria-label="window end"
        onChange={(event) => onChange({ ...range, end: Math.max(Number(event.target.value) / 1000, range.start + 0.01) })}
      />
      <span className="timeline-span">
        {(windowUs / 1000).toFixed(0)} ms window · {activeLanes} active agent(s)
        {zoomed && (
          <button type="button" className="timeline-reset" onClick={() => onChange(FULL_WINDOW)}>
            reset
          </button>
        )}
      </span>
    </div>
  );
}
