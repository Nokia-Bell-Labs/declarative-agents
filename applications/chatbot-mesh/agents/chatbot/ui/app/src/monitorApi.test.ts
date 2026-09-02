import { describe, expect, it } from "vitest";

import { HTTPError, pollDelay, statusAfterStateFailure } from "./monitorApi";

describe("statusAfterStateFailure", () => {
  it("reads a 404 as absent before first contact", () => {
    expect(statusAfterStateFailure("connecting", 404)).toBe("absent");
  });

  it("reads a 404 as absent even after a connection, when the unit is undeployed mid-session", () => {
    expect(statusAfterStateFailure("connected", 404)).toBe("absent");
  });

  it("reads a 404 as absent from an error state", () => {
    expect(statusAfterStateFailure("error", 404)).toBe("absent");
  });

  it("tolerates a non-404 failure on a connected panel as a blip", () => {
    expect(statusAfterStateFailure("connected", 500)).toBe("connected");
  });

  it("reads a non-404 failure before first contact as an error", () => {
    expect(statusAfterStateFailure("connecting", 502)).toBe("error");
  });

  it("reads a non-404 failure after an absence as an error, because the upstream now exists and refuses", () => {
    expect(statusAfterStateFailure("absent", 500)).toBe("error");
  });

  it("reads a network failure with no HTTP status as an error", () => {
    expect(statusAfterStateFailure("connecting", undefined)).toBe("error");
  });
});

describe("HTTPError", () => {
  it("carries the status the classifier reads", () => {
    const err = new HTTPError("/monitor-proxy/rag1/monitor/state", 404);
    expect(err.status).toBe(404);
    expect(err.message).toBe("/monitor-proxy/rag1/monitor/state -> HTTP 404");
  });
});

describe("pollDelay", () => {
  it("re-checks an absent agent far less often than a live one", () => {
    // The absent agent answers 404 every time, and the browser logs every one.
    // Still polling, so a unit provisioned at run time appears without a reload
    // — just not twenty times a minute.
    expect(pollDelay("absent")).toBeGreaterThan(pollDelay("connected") * 5);
  });

  it("keeps every reachable status on the live cadence", () => {
    const live = pollDelay("connected");
    expect(pollDelay("connecting")).toBe(live);
    expect(pollDelay("error")).toBe(live);
  });
});
