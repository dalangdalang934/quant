"use client";

import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";
import { endpoints, fetcher } from "../nof1";

export interface LatestDecisionAction {
  action?: string;
  symbol?: string;
  quantity?: number;
  leverage?: number;
  price?: number;
  success?: boolean;
}

export interface LatestDecisionRecord {
  timestamp: number | null;
  cycle_number?: number | null;
  summary: string;
  actions: LatestDecisionAction[];
  execution_log: string[];
}

export interface LatestDecisionRow {
  model_id: string;
  model_name?: string;
  ai_model?: string;
  records: LatestDecisionRecord[];
}

interface LatestPayload {
  latest?: LatestDecisionRow[];
}

export function useLatestDecisions(modelId?: string) {
  const key = modelId
    ? endpoints.latestDecisions(modelId)
    : endpoints.latestDecisions();
  const { data, error, isLoading } = useSWR<LatestPayload>(
    key,
    fetcher,
    {
      ...activityAwareRefresh(20_000, { hiddenInterval: 120_000 }),
    },
  );

  return {
    rows: data?.latest ?? [],
    isLoading,
    isError: !!error,
  };
}
