import { describe, expect, it } from "vitest";

import { deriveAttribution, extractSources, type ChatResponse } from "./api";

function outcome(name: string, documents: string[][] = []) {
  return { name, signal: "QueryResponded", documents };
}

function response(sources: NonNullable<NonNullable<ChatResponse["metadata"]>["sources"]>): ChatResponse {
  return { answer: "an answer", metadata: { sources } };
}

describe("deriveAttribution", () => {
  it("attributes a turn whose composed sources carry documents", () => {
    const result = deriveAttribution(
      response({
        composed: [outcome("rag0", [["chunk one", "chunk two"]]), outcome("rag1", [["chunk three"]])],
        not_selected: [],
        embedding_model_excluded: [],
        query_failed: [],
      }),
    );
    expect(result.attributed).toBe(true);
    expect(result.reasons).toEqual([]);
    expect(result.composed).toEqual([
      { name: "rag0", documents: 2 },
      { name: "rag1", documents: 1 },
    ]);
  });

  // The failure the whole change exists to catch. A rag-server bound to the
  // empty collection it opened at startup answers the query and takes part in
  // composition having retrieved nothing, and no outcome list reports a fault.
  it("refuses a turn that composed sources but retrieved no documents", () => {
    const result = deriveAttribution(response({ composed: [outcome("rag0", [[]])] }));
    expect(result.attributed).toBe(false);
    expect(result.composed).toEqual([{ name: "rag0", documents: 0 }]);
    expect(result.reasons).toEqual([{ kind: "no-documents", sources: ["rag0"] }]);
  });

  it("reports a query that failed", () => {
    const result = deriveAttribution(
      response({ composed: [], query_failed: [outcome("rag1")] }),
    );
    expect(result.attributed).toBe(false);
    expect(result.reasons).toEqual([
      { kind: "none-composed" },
      { kind: "query-failed", sources: ["rag1"] },
    ]);
  });

  it("reports an embedding-model mismatch", () => {
    const result = deriveAttribution(
      response({ composed: [], embedding_model_excluded: [outcome("rag0")] }),
    );
    expect(result.attributed).toBe(false);
    expect(result.reasons).toContainEqual({
      kind: "embedding-model-excluded",
      sources: ["rag0"],
    });
  });

  it("reports sources the router never selected", () => {
    const result = deriveAttribution(
      response({ composed: [], not_selected: ["rag0", "rag1"] }),
    );
    expect(result.attributed).toBe(false);
    expect(result.reasons).toContainEqual({ kind: "not-selected", sources: ["rag0", "rag1"] });
  });

  it("reports every populated outcome, not only the first", () => {
    const result = deriveAttribution(
      response({
        composed: [outcome("rag0", [[]])],
        query_failed: [outcome("rag1")],
        not_selected: ["rag2"],
      }),
    );
    expect(result.reasons.map((reason) => reason.kind)).toEqual([
      "no-documents",
      "query-failed",
      "not-selected",
    ]);
  });

  it("treats a response with no metadata as unattributed rather than throwing", () => {
    const result = deriveAttribution({ answer: "an answer" });
    expect(result.attributed).toBe(false);
    expect(result.composed).toEqual([]);
    expect(result.reasons).toEqual([{ kind: "none-composed" }]);
  });

  it("names an unnamed composed source by its position", () => {
    const result = deriveAttribution(
      response({ composed: [{ name: "  " }] }),
    );
    expect(result.composed).toEqual([{ name: "source 0", documents: 0 }]);
  });

  it("sums documents across every query a source answered", () => {
    const result = deriveAttribution(
      response({ composed: [outcome("rag0", [["one", "two"], ["three"]])] }),
    );
    expect(result.attributed).toBe(true);
    expect(result.composed).toEqual([{ name: "rag0", documents: 3 }]);
  });
});

describe("extractSources", () => {
  it("collects distinct bracket tokens in order", () => {
    expect(extractSources("Per [rec-1] and [rec-2], also [rec-1].")).toEqual(["rec-1", "rec-2"]);
  });

  // Inline citations are what the model wrote. Attribution is what the server
  // retrieved, and deriveAttribution owns that question.
  it("returns nothing for an answer carrying no brackets", () => {
    expect(extractSources("The corpus does not mention it.")).toEqual([]);
  });
});
