"use client";
import { useMemo } from "react";
import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";

interface NewsDigest {
  id: string;
  headline: string;
  summary: string;
  impact: string;
  sentiment: string;
  confidence?: number;
  reasoning?: string;
  source?: string;
  url?: string;
  published_at?: string;
  created_at?: string;
}

interface NewsResponse {
  digests: NewsDigest[];
  updated_at?: string;
  count?: number;
}

export type MacroNewsItem = NewsDigest & {
  timestamp: number | null;
};

async function fetcher(url: string): Promise<NewsResponse> {
  const res = await fetch(url, { cache: "no-store" });
  if (!res.ok) {
    throw new Error(`Failed to load macro news: ${res.status}`);
  }
  return res.json();
}

function toUnixSeconds(value?: string): number | null {
  if (!value) return null;
  const ts = Date.parse(value);
  if (Number.isNaN(ts)) return null;
  return Math.floor(ts / 1000);
}

export function useMacroNews() {
  const { data, error, isLoading } = useSWR<NewsResponse>(
    "/api/nof1/news/digests",
    fetcher,
    {
      ...activityAwareRefresh(60_000, { hiddenInterval: 300_000 }),
    },
  );

  const items: MacroNewsItem[] = useMemo(() => {
    const mapped = (data?.digests ?? []).map((item) => ({
      ...item,
      timestamp: toUnixSeconds(item.published_at ?? item.created_at),
    }));

    mapped.sort((a, b) => (b.timestamp ?? 0) - (a.timestamp ?? 0));
    return mapped.slice(0, 10);
  }, [data]);

  return {
    items,
    isLoading,
    isError: !!error,
  };
}
