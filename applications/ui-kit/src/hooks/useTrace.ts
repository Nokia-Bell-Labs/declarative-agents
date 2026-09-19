import { useEffect, useState } from "react";
import { useKitClient } from "../client/context";
import { fetchTrace, fetchTraceList, fetchTraces, type TraceListState, type TraceModel, type TraceState } from "../api/traceApi";

// useTrace reads one trace from the declared trace backend.
export function useTrace(backend: string, traceId: string | undefined): TraceState {
  const client = useKitClient();
  const [state, setState] = useState<TraceState>({ status: "idle" });
  useEffect(() => {
    if (!traceId) {
      setState({ status: "idle" });
      return;
    }
    let active = true;
    setState({ status: "loading" });
    void fetchTrace(client, backend, traceId).then((next) => {
      if (active) setState(next);
    });
    return () => {
      active = false;
    };
  }, [client, backend, traceId]);
  return state;
}

// useTraceList reads one page of the backend's trace list.
export function useTraceList(backend: string, pageSize: number, offset: number): TraceListState {
  const client = useKitClient();
  const [state, setState] = useState<TraceListState>({ status: "loading" });
  useEffect(() => {
    let active = true;
    setState({ status: "loading" });
    void fetchTraceList(client, backend, pageSize, offset).then((next) => {
      if (active) setState(next);
    });
    return () => {
      active = false;
    };
  }, [client, backend, pageSize, offset]);
  return state;
}

// useTraces reads several traces at once, dropping any that fail.
export function useTraces(backend: string, traceIds: string[]): Map<string, TraceModel> {
  const client = useKitClient();
  const [models, setModels] = useState<Map<string, TraceModel>>(new Map());
  const key = traceIds.join(",");
  useEffect(() => {
    let active = true;
    void fetchTraces(client, backend, key === "" ? [] : key.split(",")).then((next) => {
      if (active) setModels(next);
    });
    return () => {
      active = false;
    };
  }, [client, backend, key]);
  return models;
}
