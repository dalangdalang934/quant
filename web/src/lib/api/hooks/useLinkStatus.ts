"use client";
import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";
import { fetcher } from "../nof1";

export interface LinkStatusItem {
  key: string;
  label: string;
  linked: boolean;
  category: "data" | "diagnostic" | "control";
  endpoint?: string;
  error?: string;
}

export interface LinkStatusResponse {
  all_linked: boolean;
  items: LinkStatusItem[];
}

export function useLinkStatus() {
  const { data, error, isLoading } = useSWR<LinkStatusResponse>(
    "/api/nof1/link-status",
    fetcher,
    {
      ...activityAwareRefresh(15_000, { hiddenInterval: 45_000 }),
    },
  );

  return {
    status: data,
    isLoading,
    isError: !!error,
  };
}
