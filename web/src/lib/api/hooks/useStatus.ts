"use client";

import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";
import { endpoints, fetcher } from "../nof1";

export interface StatusRow {
  model_id: string;
  model_name?: string;
  ai_model?: string;
  exchange?: string;
  ai_provider?: string;
  is_running?: boolean;
  runtime_minutes?: number;
  call_count?: number;
  scan_interval?: string | null;
  stop_until?: string | null;
  last_reset_time?: string | null;
  total_equity?: number | null;
  total_pnl?: number | null;
  total_pnl_pct?: number | null;
  position_count?: number | null;
  margin_used_pct?: number | null;
}

interface StatusPayload {
  status?: StatusRow[];
}

export function useStatus(modelId?: string) {
  const key = modelId ? endpoints.status(modelId) : endpoints.status();
  const { data, error, isLoading } = useSWR<StatusPayload>(
    key,
    fetcher,
    {
      ...activityAwareRefresh(15_000, { hiddenInterval: 90_000 }),
    },
  );

  return {
    rows: data?.status ?? [],
    isLoading,
    isError: !!error,
  };
}
