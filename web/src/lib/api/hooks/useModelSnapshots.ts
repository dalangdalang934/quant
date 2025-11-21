"use client";
import { useAccountTotals } from "./useAccountTotals";

type Row = {
  model_id?: string;
  id?: string;
  timestamp: number;
  equity?: number | null;
  dollar_equity?: number | null;
  account_value?: number | null;
};

export function useLatestEquityMap() {
  const { data, isError, isLoading } = useAccountTotals();
  const map: Record<string, number> = {};
  const rows = (data?.accountTotals ?? []) as Row[];
  // 取每个模型最新一条记录的 dollar_equity（回退 account_value/equity）
  const latest = new Map<string, Row>();
  for (const r of rows) {
    const id = String(r.model_id ?? r.id ?? "");
    if (!id) continue;
    const ts = Number(r.timestamp ?? 0);
    const prev = latest.get(id);
    if (!prev || Number(prev.timestamp ?? 0) <= ts) latest.set(id, r);
  }
  for (const [id, r] of latest) {
    const eq = (r.dollar_equity ?? r.account_value ?? r.equity ?? 0) as number;
    map[id] = Number(eq) || 0;
  }
  return { map, isLoading, isError };
}
