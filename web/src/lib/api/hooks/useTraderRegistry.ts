"use client";

import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";
import { endpoints, fetcher } from "../nof1";

export interface TraderInfoRecord {
  model_id: string;
  model_name?: string;
  strategy?: string;
  exchange?: string;
}

interface TradersPayload {
  traders?: TraderInfoRecord[];
}

export function useTraderRegistry() {
  const { data, error, isLoading } = useSWR<TradersPayload>(
    endpoints.traders(),
    fetcher,
    {
      ...activityAwareRefresh(60_000, { hiddenInterval: 180_000 }),
    },
  );

  return {
    traders: data?.traders ?? [],
    isLoading,
    isError: !!error,
  };
}
