"use client";
import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";
import { endpoints, fetcher } from "../nof1";

export interface LearningSummary {
  model_id: string;
  model_name?: string;
  strategy?: string;
  totals: {
    total_trades: number;
    winning_trades: number;
    losing_trades: number;
    win_rate: number;
    profit_factor: number;
    sharpe_ratio: number;
    avg_win: number;
    avg_loss: number;
  };
  symbol_stats: SymbolStat[];
  recent_trades: LearningTrade[];
  best_symbol?: string | null;
  worst_symbol?: string | null;
}

export interface SymbolStat {
  symbol: string;
  total_trades: number;
  winning_trades: number;
  losing_trades: number;
  win_rate: number;
  total_pn_l: number;
  avg_pn_l: number;
}

export interface LearningTrade {
  symbol?: string;
  side?: string;
  quantity?: number;
  leverage?: number;
  open_price?: number;
  close_price?: number;
  position_value?: number;
  margin_used?: number;
  pn_l?: number;
  pn_l_pct?: number;
  duration?: string;
  open_time?: string;
  close_time?: string;
  was_stop_loss?: boolean;
}

interface LearningResponse {
  learning?: LearningSummary[];
}

export function useLearning() {
  const { data, error, isLoading } = useSWR<LearningResponse>(
    endpoints.performance(),
    fetcher,
    {
      ...activityAwareRefresh(30_000, { hiddenInterval: 90_000 }),
    },
  );

  return {
    models: data?.learning ?? [],
    isLoading,
    isError: !!error,
  };
}
