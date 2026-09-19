import { describe, expect, it } from "vitest";
import { appendHighlight, highlightFor } from "../src/api/turnActivity";

const milestones = { embed_query: "embedding the question", knowledge_query_view: "querying the retrieval units under the view" };

describe("turn activity", () => {
  it("maps milestone words to lines and stays silent otherwise", () => {
    expect(highlightFor({ command_name: "embed_query", timestamp: "t" }, milestones)).toEqual({ label: "embedding the question", at: "t" });
    expect(highlightFor({ command_name: "knowledge_query_view" }, milestones)?.label).toContain("under the view");
    expect(highlightFor({ command_name: "partition_query_results" }, milestones)).toBeUndefined();
    expect(highlightFor({}, milestones)).toBeUndefined();
  });

  it("does not repeat the previous line across a fan-out", () => {
    const feed = appendHighlight([], { label: "querying" });
    expect(appendHighlight(feed, { label: "querying" })).toHaveLength(1);
    expect(appendHighlight(feed, { label: "rendering" })).toHaveLength(2);
  });
});
