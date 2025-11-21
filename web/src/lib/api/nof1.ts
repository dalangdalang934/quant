import { fetcher } from "./client";

// Always go through our own proxy to avoid CORS issues
const local = (p: string) => `/api/nof1${p}`;

export const endpoints = {
  cryptoPrices: () => local("/crypto-prices"),
  positions: (limit = 1000) => local(`/positions?limit=${limit}`),
  trades: () => local("/trades"),
  accountTotals: (lastHourlyMarker?: number) =>
    local(
      `/account-totals${lastHourlyMarker != null ? `?lastHourlyMarker=${lastHourlyMarker}` : ""}`,
    ),
  sinceInceptionValues: () => local("/since-inception-values"),
  leaderboard: () => local("/leaderboard"),
  analytics: () => local("/analytics"),
  performance: () => local("/performance"),
  exchangeTrades: () => local("/exchange-trades"),
  status: (traderId?: string) =>
    local(`/status${traderId ? `?trader_id=${encodeURIComponent(traderId)}` : ""}`),
  statistics: (traderId?: string) =>
    local(`/statistics${traderId ? `?trader_id=${encodeURIComponent(traderId)}` : ""}`),
  latestDecisions: (traderId?: string) =>
    local(`/decisions/latest${traderId ? `?trader_id=${encodeURIComponent(traderId)}` : ""}`),
  traders: () => local("/traders"),
  conversations: () => local("/conversations"),
};

export { fetcher };
