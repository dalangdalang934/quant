"use client";
import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";
import { endpoints, fetcher } from "../nof1";
import type { TradeRow } from "./useTrades";

interface ExchangeTradesResponse {
  exchange_trades?: TradeRow[];
}

export function useExchangeTrades() {
  const { data, error, isLoading } = useSWR<ExchangeTradesResponse>(
    endpoints.exchangeTrades(),
    fetcher,
    {
      ...activityAwareRefresh(15_000),
    },
  );

  return {
    trades: data?.exchange_trades ?? [],
    isLoading,
    isError: !!error,
  };
}

