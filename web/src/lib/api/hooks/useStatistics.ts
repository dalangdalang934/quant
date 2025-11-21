"use client";

import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";
import { endpoints, fetcher } from "../nof1";

export interface StatisticsRow {
  model_id: string;
  model_name?: string;
  ai_model?: string;
  total_cycles?: number;
  successful_cycles?: number;
  failed_cycles?: number;
  total_open_positions?: number;
  total_close_positions?: number;
}

interface StatisticsPayload {
  statistics?: StatisticsRow[];
}

export function useStatistics(modelId?: string) {
  const key = modelId ? endpoints.statistics(modelId) : endpoints.statistics();
  const { data, error, isLoading } = useSWR<StatisticsPayload>(
    key,
    fetcher,
    {
      ...activityAwareRefresh(30_000, { hiddenInterval: 120_000 }),
    },
  );

  return {
    rows: data?.statistics ?? [],
    isLoading,
    isError: !!error,
  };
}
