"use client";
import type { AccountTotalsRow } from "./useAccountTotals";
import { useAccountTotals } from "./useAccountTotals";

export interface RawPositionRow {
  entry_oid: number;
  risk_usd: number;
  confidence: number;
  exit_plan: ExitPlan;
  entry_time: number; // unix seconds
  symbol: string;
  entry_price: number;
  margin: number;
  leverage: number;
  quantity: number; // positive long, negative short
  current_price: number;
  unrealized_pnl: number;
  closed_pnl?: number;
  liquidation_price?: number;
  margin_type?: string; // "isolated" | "cross"
}

export interface ExitPlan {
  profit_target?: number;
  stop_loss?: number;
  invalidation_condition?: string;
}

export interface PositionsByModel {
  id: string; // model id
  positions: Record<string, RawPositionRow>;
}

function getModelId(row: AccountTotalsRow): string {
  return String(row.model_id ?? row.id ?? "");
}

function getTimestamp(row: AccountTotalsRow): number {
  return typeof row.timestamp === "number" ? row.timestamp : Number(row.timestamp ?? 0);
}

function isRawPositionRow(value: unknown): value is RawPositionRow {
  if (!value || typeof value !== "object") return false;
  const pos = value as Record<string, unknown>;
  return (
    "entry_oid" in pos &&
    "symbol" in pos &&
    "entry_time" in pos &&
    "quantity" in pos &&
    "unrealized_pnl" in pos
  );
}

function extractPositions(row: AccountTotalsRow): Record<string, RawPositionRow> {
  const src = row.positions;
  if (!src) return {};
  const out: Record<string, RawPositionRow> = {};
  for (const [symbol, value] of Object.entries(src)) {
    if (isRawPositionRow(value)) out[symbol] = value;
  }
  return out;
}

export function usePositions() {
  const { data, isLoading, isError } = useAccountTotals();
  const rows = (data?.accountTotals ?? []) as AccountTotalsRow[];

  const positionsByModel: PositionsByModel[] = (() => {
    type SnapshotState = {
      latest: AccountTotalsRow;
      latestWithPositions?: { row: AccountTotalsRow; positions: Record<string, RawPositionRow> };
    };

    const latestById = new Map<string, SnapshotState>();
    for (const row of rows) {
      const id = getModelId(row);
      if (!id) continue;
      const ts = getTimestamp(row);
      const prev = latestById.get(id);
      const extracted = extractPositions(row);
      const state: SnapshotState = prev
        ? { ...prev }
        : {
            latest: row,
          };

      if (!prev || getTimestamp(prev.latest) <= ts) {
        state.latest = row;
      }

      if (Object.keys(extracted).length) {
        const prevPosTs = prev?.latestWithPositions ? getTimestamp(prev.latestWithPositions.row) : -Infinity;
        if (prevPosTs <= ts) {
          state.latestWithPositions = { row, positions: extracted };
        }
      }

      latestById.set(id, state);
    }
    return Array.from(latestById.entries()).map(([id, row]) => ({
      id,
      positions:
        row.latestWithPositions?.positions ??
        extractPositions(row.latest),
    }));
  })();

  return { positionsByModel, isLoading, isError };
}
