"use client";
import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";
import { endpoints, fetcher } from "../nof1";

export interface TradeRow {
  id: string;
  symbol: string;
  model_id: string;
  side: "long" | "short";
  entry_price: number | null;
  exit_price: number | null;
  quantity: number | null;
  leverage: number | null;
  entry_time: number | null;
  exit_time: number | null;
  entry_human_time?: string;
  exit_human_time?: string;
  realized_net_pnl: number | null;
  realized_gross_pnl: number | null;
  total_commission_dollars: number | null;
  pnl_pct?: number | null;
  margin_used?: number | null;
  position_value?: number | null;
  duration?: string | null;
  was_stop_loss?: boolean;
  is_partial_close?: boolean; // 是否部分平仓
  open_quantity?: number | null; // 原始开仓数量
  close_note?: string | null; // 平仓说明
  position_id?: string | null; // 仓位ID
}

type TradesResponse = { trades: TradeRow[] };

export function useTrades() {
  const { data, error, isLoading } = useSWR<TradesResponse>(
    endpoints.trades(),
    fetcher,
    {
      ...activityAwareRefresh(10_000),
    },
  );

  return {
    trades: data?.trades ?? [],
    isLoading,
    isError: !!error,
  };
}
