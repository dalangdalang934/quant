"use client";

import { useMemo } from "react";
import type { AccountTotalsRow } from "./useAccountTotals";
import { useAccountTotals } from "./useAccountTotals";

export interface SeriesPoint {
  timestamp: number; // ms epoch
  [modelId: string]: number | undefined;
}

function toMs(t: number) {
  return t > 1e12 ? Math.floor(t) : Math.floor(t * 1000);
}

function ingestTotals(
  map: Map<number, SeriesPoint>,
  items: AccountTotalsRow[],
) {
  for (const it of items) {
    const modelId = it?.model_id ?? it?.id;
    if (!modelId || typeof it.timestamp !== "number") continue;
    const ts = toMs(it.timestamp);
    const v = it.dollar_equity ?? it.equity ?? it.account_value;
    if (typeof v !== "number") continue;
    const p = map.get(ts) || { timestamp: ts };
    p[modelId] = v;
    map.set(ts, p);
  }
}

export function useAccountValueSeries() {
  const { data, isLoading, isError } = useAccountTotals();

  const merged = useMemo(() => {
    const rows = (data?.accountTotals ?? []) as AccountTotalsRow[];
    if (!rows.length) return [] as SeriesPoint[];
    const tmp = new Map<number, SeriesPoint>();
    ingestTotals(tmp, rows);
    return Array.from(tmp.values()).sort((a, b) => a.timestamp - b.timestamp);
  }, [data]);

  const idsSet = new Set<string>();
  for (const p of merged)
    for (const k of Object.keys(p)) if (k !== "timestamp") idsSet.add(k);

  // If still only 1 point, synthesize a baseline one minute earlier
  let out = merged;
  if (out.length === 1) {
    const only = out[0];
    const prevTs = only.timestamp - 60_000;
    const synth: SeriesPoint = { timestamp: prevTs };
    for (const key of Object.keys(only)) {
      if (key === "timestamp") continue;
      const value = only[key];
      if (typeof value === "number") synth[key] = value;
    }
    out = [synth, only];
  }

  return {
    series: out,
    modelIds: Array.from(idsSet),
    isLoading,
    isError,
  };
}
