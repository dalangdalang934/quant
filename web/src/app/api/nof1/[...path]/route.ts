import { NextRequest, NextResponse } from "next/server";

export const runtime = "nodejs";

const RAW_BASE = process.env.NOF1_API_BASE_URL || "http://localhost:8080";
const BASE = RAW_BASE.endsWith("/") ? RAW_BASE.slice(0, -1) : RAW_BASE;
const API_BASE = BASE.endsWith("/api") ? BASE : `${BASE}/api`;

const DEFAULT_HEADERS = {
  "access-control-allow-origin": "*",
  "cache-control": "no-store",
};

type CacheState<T> = {
  data: T | null;
  lastUpdated: number;
  lastError?: string;
  refreshing: boolean;
  promise?: Promise<void>;
  interval?: NodeJS.Timeout;
};

const ACCOUNT_TOTALS_REFRESH_MS = Number(
  process.env.ACCOUNT_TOTALS_REFRESH_MS ?? "1000",
);
const STATUS_REFRESH_MS = Number(
  process.env.STATUS_REFRESH_MS ?? "1000",
);
const STATISTICS_REFRESH_MS = Number(
  process.env.STATISTICS_REFRESH_MS ?? "1500",
);

const globalState = globalThis as any;

if (!globalState.__nof1AccountTotalsCache) {
  globalState.__nof1AccountTotalsCache = {
    data: null,
    lastUpdated: 0,
    refreshing: false,
  } as CacheState<AccountTotalsPayload>;
}
const accountTotalsCache =
  globalState.__nof1AccountTotalsCache as CacheState<AccountTotalsPayload>;

if (!globalState.__nof1StatusCache) {
  globalState.__nof1StatusCache = {
    data: null,
    lastUpdated: 0,
    refreshing: false,
  } as CacheState<StatusPayload>;
}
const statusCache =
  globalState.__nof1StatusCache as CacheState<StatusPayload>;

if (!globalState.__nof1StatisticsCache) {
  globalState.__nof1StatisticsCache = {
    data: null,
    lastUpdated: 0,
    refreshing: false,
  } as CacheState<StatisticsPayload>;
}
const statisticsCache =
  globalState.__nof1StatisticsCache as CacheState<StatisticsPayload>;

if (!accountTotalsCache.interval) {
  startScheduler(
    accountTotalsCache,
    ACCOUNT_TOTALS_REFRESH_MS,
    buildAccountTotalsPayload,
  );
}
if (!statusCache.interval) {
  startScheduler(statusCache, STATUS_REFRESH_MS, buildStatusPayload);
}
if (!statisticsCache.interval) {
  startScheduler(
    statisticsCache,
    STATISTICS_REFRESH_MS,
    buildStatisticsPayload,
  );
}

function startScheduler<T>(
  cache: CacheState<T>,
  refreshMs: number,
  builder: () => Promise<T>,
) {
  const intervalMs = Math.max(refreshMs, 250);
  const tick = () => {
    refreshCache(cache, builder).catch(() => {});
  };
  cache.interval = setInterval(tick, intervalMs);
  if (typeof cache.interval.unref === "function") {
    cache.interval.unref();
  }
  tick();
}

async function refreshCache<T>(
  cache: CacheState<T>,
  builder: () => Promise<T>,
) {
  if (cache.refreshing) {
    return cache.promise ?? Promise.resolve();
  }
  cache.refreshing = true;
  const promise = builder()
    .then((data) => {
      cache.data = data;
      cache.lastUpdated = Date.now();
      cache.lastError = undefined;
    })
    .catch((err) => {
      cache.lastError =
        err instanceof Error ? err.message : String(err);
      throw err;
    })
    .finally(() => {
      cache.refreshing = false;
      cache.promise = undefined;
    });
  cache.promise = promise;
  return promise;
}

function jsonResponse(data: any, init?: ResponseInit) {
  return NextResponse.json(data, {
    ...init,
    headers: {
      ...DEFAULT_HEADERS,
      ...(init?.headers || {}),
    },
  });
}

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    cache: "no-store",
    ...init,
  });
  if (!res.ok) {
    throw new Error(`Request failed ${res.status}: ${await res.text()}`);
  }
  return (await res.json()) as T;
}

interface CompetitionResponse {
  count: number;
  traders: Array<{
    trader_id: string;
    trader_name: string;
    strategy?: string;
    total_equity?: number;
    total_pnl?: number;
    total_pnl_pct?: number;
    position_count?: number;
    margin_used_pct?: number;
    call_count?: number;
    is_running?: boolean;
    runtime_minutes?: number;
  }>;
}

interface AccountResponse {
  total_equity?: number;
  total_unrealized_pnl?: number;
  total_pnl?: number;
  total_pnl_pct?: number;
  available_balance?: number;
  wallet_balance?: number;
  position_count?: number;
  margin_used?: number;
  margin_used_pct?: number;
  initial_balance?: number;
}

interface PositionResponse {
  symbol: string;
  side?: string;
  entry_price?: number;
  mark_price?: number;
  quantity?: number;
  leverage?: number;
  unrealized_pnl?: number;
  unrealized_pnl_pct?: number;
  liquidation_price?: number;
  margin_used?: number;
  margin_type?: string;
  order_id?: number;
  risk_usd?: number;
  confidence?: number;
  entry_time?: number;
  exit_plan?: Record<string, unknown>;
  stop_loss?: number;
  take_profit?: number;
}

interface EquityHistoryPoint {
  timestamp: string | number;
  total_equity?: number;
  total_pnl_pct?: number;
}

interface LinkStatusItemPayload {
  key: string;
  label: string;
  linked: boolean;
  category: "data" | "diagnostic" | "control";
  endpoint?: string;
  error?: string;
}

interface LinkStatusPayload {
  all_linked: boolean;
  items: LinkStatusItemPayload[];
}

interface DecisionAction {
  action?: string;
  symbol?: string;
  price?: number;
  quantity?: number;
  leverage?: number;
  position_id?: string;
}

interface DecisionRecord {
  timestamp?: string;
  trader_id?: string;
  trader_name?: string;
  input_prompt?: string;
  cot_trace?: string;
  cot_trace_summary?: string;
  decision_json?: string;
  decisions?: DecisionAction[];
  execution_log?: string[];
}

interface SymbolPerformanceEntry {
  symbol?: string;
  total_trades?: number;
  winning_trades?: number;
  losing_trades?: number;
  win_rate?: number;
  total_pn_l?: number;
  avg_pn_l?: number;
}

interface PerformanceAPIResponse {
  total_trades?: number;
  winning_trades?: number;
  losing_trades?: number;
  win_rate?: number;
  avg_win?: number;
  avg_loss?: number;
  profit_factor?: number;
  sharpe_ratio?: number;
  recent_trades?: TradeOutcome[];
  symbol_stats?: Record<string, SymbolPerformanceEntry>;
  best_symbol?: string;
  worst_symbol?: string;
}

interface PositionSnapshot {
  entry_oid: number;
  risk_usd: number;
  confidence: number;
  exit_plan: Record<string, unknown>;
  entry_time: number;
  symbol: string;
  entry_price: number;
  margin: number;
  leverage: number;
  quantity: number;
  current_price: number;
  unrealized_pnl: number;
  liquidation_price?: number;
  margin_type?: string;
  side: string;
}

interface AccountTotalsRow {
  model_id: string;
  id: string;
  timestamp: number;
  dollar_equity: number;
  account_value: number;
  return_pct: number;
  total_pnl?: number;
  unrealized_pnl?: number;
  positions?: Record<string, PositionSnapshot>;
  initial_balance?: number;
}

type AccountTotalsPayload = { accountTotals: AccountTotalsRow[] };

interface StatusRow {
  model_id: string;
  model_name?: string;
  strategy?: string;
  strategy_label?: string | null;
  exchange?: string | null;
  is_running: boolean;
  runtime_minutes: number;
  call_count: number;
  scan_interval?: string | null;
  stop_until?: string | null;
  last_reset_time?: string | null;
  total_equity?: number | null;
  total_pnl?: number | null;
  total_pnl_pct?: number | null;
  position_count?: number | null;
  margin_used_pct?: number | null;
}

type StatusPayload = { status: StatusRow[] };

interface StatisticsRecord {
  total_cycles?: number;
  successful_cycles?: number;
  failed_cycles?: number;
  total_open_positions?: number;
  total_close_positions?: number;
}

interface StatisticsRow extends StatisticsRecord {
  model_id: string;
  model_name?: string;
  strategy?: string;
}

type StatisticsPayload = { statistics: StatisticsRow[] };

interface TradersResponse {
  traders?: Array<{
    trader_id: string;
    trader_name?: string;
    strategy?: string;
  }>;
}

interface TradeOutcome {
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
  is_partial_close?: boolean; // 是否部分平仓
  open_quantity?: number; // 原始开仓数量
  close_note?: string;
}

interface ExchangeTradesAPIResponse {
  exchange_trades?: TradeOutcome[];
}

async function handleAccountTotals(req: NextRequest) {
  const force = req.nextUrl.searchParams.get("force");
  if (force === "1") {
    try {
      await refreshCache(accountTotalsCache, buildAccountTotalsPayload);
    } catch (error) {
      return jsonResponse(
        {
          accountTotals: accountTotalsCache.data?.accountTotals ?? [],
          stale: true,
          error:
            accountTotalsCache.lastError ||
            (error instanceof Error ? error.message : String(error)),
        },
        { status: 502 },
      );
    }
  }

  if (!accountTotalsCache.data) {
    await refreshCache(accountTotalsCache, buildAccountTotalsPayload).catch(
      () => {},
    );
  }

  if (!accountTotalsCache.data) {
    const status = accountTotalsCache.lastError ? 502 : 503;
    return jsonResponse(
      {
        accountTotals: [],
        stale: true,
        error:
          accountTotalsCache.lastError ??
          "account totals are still loading, please retry shortly",
      },
      { status },
    );
  }

  const ageMs = Date.now() - accountTotalsCache.lastUpdated;
  return jsonResponse(accountTotalsCache.data, {
    headers: {
      "x-cache-age": String(ageMs),
      "x-cache-refreshing": accountTotalsCache.refreshing ? "1" : "0",
    },
  });
}

async function buildAccountTotalsPayload() {
  const competition = await fetchJSON<CompetitionResponse>(`/competition`).catch(
    () => ({ traders: [], count: 0 }),
  );
  const traders = competition.traders ?? [];

  const rows: AccountTotalsRow[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const traderId = trader.trader_id;
      try {
        const [account, positions, history] = await Promise.all([
          fetchJSON<AccountResponse>(`/account?trader_id=${encodeURIComponent(traderId)}`).catch<AccountResponse>(() => ({})),
          fetchJSON<PositionResponse[]>(`/positions?trader_id=${encodeURIComponent(traderId)}`).catch<PositionResponse[]>(() => []),
          fetchJSON<EquityHistoryPoint[]>(`/equity-history?trader_id=${encodeURIComponent(traderId)}&limit=720`).catch<EquityHistoryPoint[]>(() => []),
        ]);

        const nowTs = Date.now();
        const historyList = Array.isArray(history) ? history : [];
        const historyRows = historyList
          .filter((p) => p && p.timestamp != null)
          .map((p) => {
            const ts = typeof p.timestamp === "number" ? p.timestamp : Date.parse(p.timestamp);
            return {
              model_id: traderId,
              id: traderId,
              timestamp: Number.isFinite(ts) ? ts : nowTs,
              dollar_equity: p.total_equity ?? trader.total_equity ?? account.total_equity ?? 0,
              account_value: p.total_equity ?? trader.total_equity ?? account.total_equity ?? 0,
              return_pct: p.total_pnl_pct ?? trader.total_pnl_pct ?? account.total_pnl_pct ?? 0,
            };
          });
        rows.push(...historyRows);

        const positionMap: Record<string, PositionSnapshot> = {};
        const posList = Array.isArray(positions) ? positions : [];
        posList.forEach((pos, index) => {
          if (!pos || !pos.symbol) return;
          const rawQty = Number(pos.quantity ?? 0);
          const symbol = String(pos.symbol).toUpperCase();
          const sideText = String(pos.side ?? "").toUpperCase();
          const qty = sideText.includes("SHORT")
            ? -Math.abs(rawQty)
            : Math.abs(rawQty);
          const entryPrice = Number(pos.entry_price ?? pos.mark_price ?? 0);
          const markPrice = Number(pos.mark_price ?? pos.entry_price ?? 0);

          const stopLossFromExchange = pos.stop_loss ?? undefined;
          const takeProfitFromExchange = pos.take_profit ?? undefined;

          const exitPlan: Record<string, unknown> = {
            ...(pos.exit_plan ?? {}),
          };
          if (stopLossFromExchange) {
            exitPlan.stop_loss = stopLossFromExchange;
          }
          if (takeProfitFromExchange) {
            exitPlan.profit_target = takeProfitFromExchange;
          }

          let parsedEntryId = Number.NaN;
          if (typeof pos.order_id === "number") {
            parsedEntryId = pos.order_id;
          } else if (typeof pos.order_id === "string") {
            const candidate = Number(pos.order_id);
            if (Number.isFinite(candidate)) {
              parsedEntryId = candidate;
            }
          }

          positionMap[symbol] = {
            entry_oid:
              Number.isFinite(parsedEntryId)
                ? parsedEntryId
                : index,
            risk_usd:
              typeof pos.risk_usd === "number"
                ? pos.risk_usd
                : Math.abs(rawQty) * Math.abs(markPrice),
            confidence: Number(pos.confidence ?? 0),
            exit_plan: exitPlan,
            entry_time: pos.entry_time ?? Math.floor(nowTs / 1000),
            symbol,
            entry_price: entryPrice,
            margin: Number(pos.margin_used ?? 0),
            leverage: Number(pos.leverage ?? 0),
            quantity: qty,
            current_price: markPrice,
            unrealized_pnl: Number(pos.unrealized_pnl ?? 0),
            liquidation_price:
              typeof pos.liquidation_price === "number"
                ? pos.liquidation_price
                : undefined,
            margin_type: pos.margin_type ?? "isolated",
            side: sideText || (qty >= 0 ? "LONG" : "SHORT"),
          };
        });

        rows.push({
          model_id: traderId,
          id: traderId,
          timestamp: nowTs,
          dollar_equity:
            Number(account.total_equity ?? trader.total_equity ?? 0),
          account_value:
            Number(account.total_equity ?? trader.total_equity ?? 0),
          return_pct: Number(
            account.total_pnl_pct ?? trader.total_pnl_pct ?? 0,
          ),
          total_pnl: Number(account.total_pnl ?? trader.total_pnl ?? 0),
          unrealized_pnl: Number(account.total_unrealized_pnl ?? 0),
          positions: positionMap,
          initial_balance:
            typeof account.initial_balance === "number"
              ? account.initial_balance
              : undefined,
        });
      } catch (error) {
        console.error("Failed to build account row", error);
      }
    }),
  );

  return { accountTotals: rows };
}

async function buildStatusPayload(): Promise<StatusPayload> {
  const competition = await fetchJSON<CompetitionResponse>(`/competition`).catch(
    () => ({ traders: [], count: 0 }),
  );
  const traders = competition.traders ?? [];

  const rows: StatusRow[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const id = trader.trader_id;
      try {
        const status = await fetchJSON<Record<string, unknown>>(
          `/status?trader_id=${encodeURIComponent(id)}`,
        ).catch<Record<string, unknown> | null>(() => null);
        if (!status) return;

        const statusRecord = status as Record<string, unknown>;

        const exchange =
          typeof statusRecord["exchange"] === "string"
            ? (statusRecord["exchange"] as string)
            : null;
        const strategyLabel =
          typeof statusRecord["strategy_label"] === "string"
            ? (statusRecord["strategy_label"] as string)
            : null;
        const scanInterval =
          typeof statusRecord["scan_interval"] === "string"
            ? (statusRecord["scan_interval"] as string)
            : null;
        const stopUntil =
          typeof statusRecord["stop_until"] === "string"
            ? (statusRecord["stop_until"] as string)
            : null;
        const lastReset =
          typeof statusRecord["last_reset_time"] === "string"
            ? (statusRecord["last_reset_time"] as string)
            : null;

        rows.push({
          model_id: id,
          model_name: trader.trader_name,
          strategy: trader.strategy || strategyLabel || trader.trader_name,
          strategy_label: strategyLabel,
          exchange,
          is_running: Boolean(
            statusRecord["is_running"] ?? trader.is_running ?? false,
          ),
          runtime_minutes: Number(
            statusRecord["runtime_minutes"] ??
              trader.runtime_minutes ??
              0,
          ),
          call_count: Number(
            statusRecord["call_count"] ?? trader.call_count ?? 0,
          ),
          scan_interval: scanInterval,
          stop_until: stopUntil,
          last_reset_time: lastReset,
          total_equity: numberOrNull(
            statusRecord["total_equity"] ?? trader.total_equity,
          ),
          total_pnl: numberOrNull(
            statusRecord["total_pnl"] ?? trader.total_pnl,
          ),
          total_pnl_pct: numberOrNull(
            statusRecord["total_pnl_pct"] ?? trader.total_pnl_pct,
          ),
          position_count: Number(
            statusRecord["position_count"] ?? trader.position_count ?? 0,
          ),
          margin_used_pct: numberOrNull(
            statusRecord["margin_used_pct"] ?? trader.margin_used_pct,
          ),
        });
      } catch (error) {
        console.error("Failed to build status row", error);
      }
    }),
  );

  return { status: rows };
}

async function buildStatisticsPayload(): Promise<StatisticsPayload> {
  const competition = await fetchJSON<CompetitionResponse>(`/competition`).catch(
    () => ({ traders: [], count: 0 }),
  );
  const traders = competition.traders ?? [];

  const rows: StatisticsRow[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const id = trader.trader_id;
      try {
        const stats = await fetchJSON<StatisticsRecord>(
          `/statistics?trader_id=${encodeURIComponent(id)}`,
        ).catch<StatisticsRecord | null>(() => null);
        if (!stats) return;
        rows.push({
          model_id: id,
          model_name: trader.trader_name,
          strategy: trader.strategy,
          total_cycles: Number(stats.total_cycles ?? 0),
          successful_cycles: Number(stats.successful_cycles ?? 0),
          failed_cycles: Number(stats.failed_cycles ?? 0),
          total_open_positions: Number(stats.total_open_positions ?? 0),
          total_close_positions: Number(stats.total_close_positions ?? 0),
        });
      } catch (error) {
        console.error("Failed to build statistics row", error);
      }
    }),
  );

  return { statistics: rows };
}

async function handleTrades() {
  const competition = await fetchJSON<CompetitionResponse>(`/competition`).catch(() => ({ traders: [], count: 0 }));
  const traders = competition.traders ?? [];
  const trades: any[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const traderId = trader.trader_id;
      const perf = await fetchJSON<PerformanceAPIResponse>(`/performance?trader_id=${encodeURIComponent(traderId)}`).catch<PerformanceAPIResponse | null>(() => null);
      const recent = perf?.recent_trades ?? [];

      recent.forEach((item, idx) => {
        const symbol = String(item.symbol ?? "").toUpperCase();
        if (!symbol) return;
        const entryTs = toUnixSeconds((item as any).entry_time ?? item.open_time, idx);
        const exitTs = toUnixSeconds((item as any).exit_time ?? item.close_time, idx);

        trades.push({
          id: `${traderId}-${exitTs || entryTs || Date.now()}-${idx}`,
          symbol,
          model_id: traderId,
          side: normalizeTradeSide(item.side),
          entry_price: numberOrNull((item as any).entry_price ?? item.open_price),
          exit_price: numberOrNull((item as any).exit_price ?? item.close_price),
          quantity: numberOrNull(item.quantity),
          leverage: numberOrNull(item.leverage),
          entry_time: entryTs,
          exit_time: exitTs,
          realized_net_pnl: numberOrNull((item as any).realized_net_pnl ?? item.pn_l),
          realized_gross_pnl: numberOrNull((item as any).realized_gross_pnl ?? (item as any).realized_net_pnl ?? item.pn_l),
          total_commission_dollars: numberOrNull((item as any).total_commission_dollars),
          pnl_pct: numberOrNull((item as any).pnl_pct ?? item.pn_l_pct),
          margin_used: numberOrNull((item as any).margin_used ?? item.margin_used),
          position_value: numberOrNull((item as any).position_value ?? item.position_value),
          duration: item.duration || null,
          was_stop_loss: !!item.was_stop_loss,
          is_partial_close: item.is_partial_close ?? false,
          open_quantity: numberOrNull((item as any).open_quantity ?? item.open_quantity),
          close_note: item.close_note ?? null,
          position_id: (item as any).position_id ?? null,
        });
      });
    }),
  );

  trades.sort((a, b) => {
    const ta = Number(a.exit_time || a.entry_time || 0);
    const tb = Number(b.exit_time || b.entry_time || 0);
    return tb - ta;
  });

  return jsonResponse({ trades });
}

async function handleExchangeTrades(req: NextRequest) {
  const search = req.nextUrl.searchParams;
  const traderId = search.get("trader_id");
  const traders = await fetchCompetitionTraders(traderId || undefined);
  if (!traders.length) {
    return jsonResponse({ exchange_trades: [] });
  }

  const rows: any[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const id = trader.trader_id;
      const resp = await fetchJSON<ExchangeTradesAPIResponse>(`/exchange-trades?trader_id=${encodeURIComponent(id)}`).catch<ExchangeTradesAPIResponse | null>(() => null);
      const history = resp?.exchange_trades ?? [];

      history.forEach((item, idx) => {
        const symbol = String(item.symbol ?? "").toUpperCase();
        if (!symbol) return;
        const entryTs = toUnixSeconds((item as any).entry_time ?? item.open_time, idx);
        const exitTs = toUnixSeconds((item as any).exit_time ?? item.close_time, idx);

        rows.push({
          id: `${id}-exchange-${exitTs || entryTs || Date.now()}-${idx}`,
          symbol,
          model_id: id,
          side: normalizeTradeSide(item.side),
          entry_price: numberOrNull((item as any).entry_price ?? item.open_price),
          exit_price: numberOrNull((item as any).exit_price ?? item.close_price),
          quantity: numberOrNull(item.quantity),
          leverage: numberOrNull(item.leverage),
          entry_time: entryTs,
          exit_time: exitTs,
          realized_net_pnl: numberOrNull((item as any).realized_net_pnl ?? item.pn_l),
          realized_gross_pnl: numberOrNull((item as any).realized_gross_pnl ?? (item as any).realized_net_pnl ?? item.pn_l),
          total_commission_dollars: numberOrNull((item as any).total_commission_dollars),
          pnl_pct: numberOrNull((item as any).pnl_pct ?? item.pn_l_pct),
          margin_used: numberOrNull((item as any).margin_used ?? item.margin_used),
          position_value: numberOrNull((item as any).position_value ?? item.position_value),
          duration: item.duration || null,
          was_stop_loss: !!item.was_stop_loss,
          is_partial_close: item.is_partial_close ?? false,
          open_quantity: numberOrNull((item as any).open_quantity ?? item.open_quantity),
          close_note: item.close_note ?? null,
        });
      });
    }),
  );

  rows.sort((a, b) => {
    const ta = Number(a.exit_time || a.entry_time || 0);
    const tb = Number(b.exit_time || b.entry_time || 0);
    return tb - ta;
  });

  return jsonResponse({ exchange_trades: rows });
}

async function handleConversations() {
  const competition = await fetchJSON<CompetitionResponse>(`/competition`).catch(() => ({ traders: [], count: 0 }));
  const traders = competition.traders ?? [];
  const cards: any[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const traderId = trader.trader_id;
      const decisions = await fetchJSON<DecisionRecord[]>(`/decisions?trader_id=${encodeURIComponent(traderId)}`).catch<DecisionRecord[]>(() => []);
      if (!decisions || !Array.isArray(decisions)) {
        return;
      }
      decisions.forEach((decision, idx) => {
        const ts = parseTimestamp(decision.timestamp, idx);
        const summary = pickSummary(decision);
        const userPrompt = cleanString(decision.input_prompt || (decision as any).prompt || "");
        const publicPrompt = maskPrompt(traderId, userPrompt || summary || "");
        const cotTrace = decision.cot_trace ?? decision.cot_trace_summary ?? decision.decision_json ?? "";
        const llmResponse = buildDecisionMap(decision);

        if (!summary && !userPrompt && !cotTrace && !Object.keys(llmResponse).length) {
          return;
        }

        cards.push({
          model_id: traderId,
          timestamp: ts,
          summary,
          user_prompt: publicPrompt,
          cot_trace: cotTrace,
          llm_response: llmResponse,
        });
      });
    }),
  );

  cards.sort((a, b) => Number(b.timestamp || 0) - Number(a.timestamp || 0));

  return jsonResponse({ conversations: cards });
}

async function fetchCompetitionTraders(targetId?: string) {
  const competition = await fetchJSON<CompetitionResponse>(`/competition`).catch(() => ({ traders: [], count: 0 }));
  const traders = competition.traders ?? [];
  if (!targetId) return traders;
  return traders.filter((t) => t.trader_id === targetId);
}

async function handleStatus(req: NextRequest) {
  const force = req.nextUrl.searchParams.get("force");
  if (force === "1") {
    await refreshCache(statusCache, buildStatusPayload).catch((error) => {
      console.error("Failed to refresh status cache", error);
    });
  }

  if (!statusCache.data) {
    await refreshCache(statusCache, buildStatusPayload).catch(() => {});
  }

  if (!statusCache.data) {
    const status = statusCache.lastError ? 502 : 503;
    return jsonResponse(
      {
        status: [],
        stale: true,
        error:
          statusCache.lastError ??
          "status data is still loading, please retry shortly",
      },
      { status },
    );
  }

  const traderId = req.nextUrl.searchParams.get("trader_id");
  const rows = traderId
    ? statusCache.data.status.filter((row) => row.model_id === traderId)
    : statusCache.data.status;

  const ageMs = Date.now() - statusCache.lastUpdated;
  return jsonResponse(
    {
      status: rows,
    },
    {
      headers: {
        "x-cache-age": String(ageMs),
        "x-cache-refreshing": statusCache.refreshing ? "1" : "0",
      },
    },
  );
}

async function handleStatistics(req: NextRequest) {
  const force = req.nextUrl.searchParams.get("force");
  if (force === "1") {
    await refreshCache(statisticsCache, buildStatisticsPayload).catch(
      (error) => {
        console.error("Failed to refresh statistics cache", error);
      },
    );
  }

  if (!statisticsCache.data) {
    await refreshCache(statisticsCache, buildStatisticsPayload).catch(() => {});
  }

  if (!statisticsCache.data) {
    const status = statisticsCache.lastError ? 502 : 503;
    return jsonResponse(
      {
        statistics: [],
        stale: true,
        error:
          statisticsCache.lastError ??
          "statistics data is still loading, please retry shortly",
      },
      { status },
    );
  }

  const traderId = req.nextUrl.searchParams.get("trader_id");
  const rows = traderId
    ? statisticsCache.data.statistics.filter((row) => row.model_id === traderId)
    : statisticsCache.data.statistics;

  const ageMs = Date.now() - statisticsCache.lastUpdated;
  return jsonResponse(
    { statistics: rows },
    {
      headers: {
        "x-cache-age": String(ageMs),
        "x-cache-refreshing": statisticsCache.refreshing ? "1" : "0",
      },
    },
  );
}

async function handleLatestDecisions(req: NextRequest) {
  const search = req.nextUrl.searchParams;
  const traderId = search.get("trader_id");
  const traders = await fetchCompetitionTraders(traderId || undefined);
  if (!traders.length) return jsonResponse({ latest: [] });

  const rows: any[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const id = trader.trader_id;
      try {
        const decisions = await fetchJSON<DecisionRecord[]>(`/decisions/latest?trader_id=${encodeURIComponent(id)}`).catch<DecisionRecord[]>(() => []);
        if (!decisions.length) return;
          rows.push({
            model_id: id,
            model_name: trader.trader_name,
            strategy: trader.strategy,
            records: decisions.map((decision) => ({
            timestamp: parseTimestamp(decision.timestamp),
            cycle_number: (decision as any).cycle_number ?? null,
            summary: pickSummary(decision),
            actions: decision.decisions ?? [],
            execution_log: decision.execution_log ?? [],
          })),
        });
      } catch (error) {
        console.error("Failed to load latest decisions", error);
      }
    }),
  );

  return jsonResponse({ latest: rows });
}

async function handleTraders() {
  try {
    const traders = await fetchJSON<TradersResponse>(`/traders`).catch(() => ({ traders: [] }));
    return jsonResponse(traders ?? { traders: [] });
  } catch (error) {
    console.error("Failed to load traders", error);
    return jsonResponse({ traders: [] }, { status: 500 });
  }
}

async function handlePerformance(req: NextRequest) {
  const search = req.nextUrl.searchParams;
  const traderId = search.get("trader_id");
  if (traderId) {
    const target = `${API_BASE}/performance?trader_id=${encodeURIComponent(traderId)}`;
    const upstream = await fetch(target, { cache: "no-store" });
    const data = await upstream.json().catch(() => ({}));
    return jsonResponse(data, { status: upstream.status });
  }

  const competition = await fetchJSON<CompetitionResponse>(`/competition`).catch(() => ({ traders: [], count: 0 }));
  const traders = competition.traders ?? [];
  const rows: any[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const traderId = trader.trader_id;
      try {
        const perf = await fetchJSON<PerformanceAPIResponse>(`/performance?trader_id=${encodeURIComponent(traderId)}`).catch<PerformanceAPIResponse | null>(() => null);
        if (!perf) return;
          rows.push({
            model_id: traderId,
            model_name: trader.trader_name,
            strategy: trader.strategy,
            totals: {
            total_trades: perf.total_trades ?? 0,
            winning_trades: perf.winning_trades ?? 0,
            losing_trades: perf.losing_trades ?? 0,
            win_rate: perf.win_rate ?? 0,
            profit_factor: perf.profit_factor ?? 0,
            sharpe_ratio: perf.sharpe_ratio ?? 0,
            avg_win: perf.avg_win ?? 0,
            avg_loss: perf.avg_loss ?? 0,
          },
          symbol_stats: normalizeSymbolStats(perf.symbol_stats),
          recent_trades: (perf.recent_trades ?? []).slice(0, 15),
          best_symbol: perf.best_symbol ?? null,
          worst_symbol: perf.worst_symbol ?? null,
        });
      } catch (error) {
        console.error("Failed to load performance", error);
      }
    }),
  );

  return jsonResponse({ learning: rows });
}

function parseTimestamp(value?: string, fallbackIdx?: number): number {
  if (!value) return Date.now() - (fallbackIdx ?? 0) * 1000;
  const parsed = Date.parse(value);
  if (Number.isFinite(parsed)) return parsed;
  const num = Number(value);
  if (Number.isFinite(num)) return num > 1e12 ? num : num * 1000;
  return Date.now();
}

function cleanString(input?: string): string {
  if (!input) return "";
  return String(input).replace(/\r/g, "").trim();
}

const FUN_MASK_MESSAGES = [
  "今日任务：不要剧透真实战术，就说自己忙着研究猫咪K线图吧。",
  "表面上我在讨论早餐吃什么，其实心里在排兵布阵。",
  "请告诉旁观者：我是来写脱口秀的，交易只是顺手。",
  "如果有人偷看，就让他们以为我在研究土味情话指数。",
  "此刻的我正纠结要不要把奶茶珍珠当作支撑位来分析。",
  "外界版本：我正在学习如何用抖音热梗预测行情。",
  "机密遮罩：只准看到段子，真实策略锁在保险柜里。",
  "告诉所有围观群众：这份prompt经过防泄密认证，只有笑点没有情报。",
];

function maskPrompt(traderId: string, original: string): string {
  const seed = `${traderId}|${original.length}`;
  const base = Math.abs(hashString(seed));
  const primary = FUN_MASK_MESSAGES[base % FUN_MASK_MESSAGES.length];
  const secondary = FUN_MASK_MESSAGES[(base + 3) % FUN_MASK_MESSAGES.length];
  const jokes = Array.from(new Set([primary, secondary]))
    .map((line) => `- ${line}`)
    .join("\n");
  return `🤡 模型公开日记（戏谑版）\n${jokes}\n\n（真实逻辑已加密，仅内部可见。）`;
}

function hashString(value: string): number {
  let hash = 0;
  for (let i = 0; i < value.length; i++) {
    hash = (hash << 5) - hash + value.charCodeAt(i);
    hash |= 0;
  }
  return hash;
}

function pickSummary(decision: DecisionRecord): string {
  const raw =
    (decision as any).cot_trace_summary ||
    (decision as any).summary ||
    (decision.cot_trace && typeof decision.cot_trace === "string" && decision.cot_trace) ||
    (decision.execution_log && decision.execution_log.join("\n")) ||
    "";
  const text = cleanString(raw as string);
  if (!text) return "";
  if (text.length <= 360) return text;
  return `${text.slice(0, 357)}...`;
}

function buildDecisionMap(decision: DecisionRecord): Record<string, any> {
  const map: Record<string, any> = {};

  const parseArray = () => {
    const raw = decision.decision_json;
    if (!raw) return [];
    try {
      const parsed = typeof raw === "string" ? JSON.parse(raw) : raw;
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  };

  const fromJson = parseArray();
  const source = fromJson.length ? fromJson : (decision.decisions ?? []);

  source.forEach((item: any) => {
    if (!item) return;
    const k = String(item.symbol || item.coin || "").trim();
    if (!k) return;
    const symbol = k.toUpperCase();
    const signal = normalizeSignal(item.action || item.signal);
    const entry: Record<string, any> = {
      signal,
      leverage: item.leverage != null ? Number(item.leverage) : undefined,
      profit_target:
        item.profit_target ?? item.target ?? item.take_profit ?? item.take_profit_price,
      stop_loss: item.stop_loss ?? item.stop_loss_price,
      risk_usd: item.risk_usd ?? item.risk,
      invalidation_condition: item.invalidation_condition,
      confidence: item.confidence != null ? Number(item.confidence) : undefined,
      quantity: item.quantity != null ? Number(item.quantity) : undefined,
      price: item.price != null ? Number(item.price) : undefined,
    };
    map[symbol] = entry;
  });

  return map;
}

function normalizeSignal(signal?: string): string | undefined {
  if (!signal) return undefined;
  const k = signal.toString().toLowerCase();
  if (k.includes("long") || k.includes("buy")) return "long";
  if (k.includes("short") || k.includes("sell")) return "short";
  if (k.includes("hold") || k.includes("wait")) return "hold";
  return signal;
}

// 获取默认traderId
async function getDefaultTraderId() {
  // 先尝试从环境变量获取
  const envTraderId = process.env.NEXT_PUBLIC_DEFAULT_TRADER_ID;
  if (envTraderId) return envTraderId;
  
  // 否则从traders列表获取第一个
  const fallbackId = "binance_deepseek";
  try {
    const traders = await fetchJSON<any>("/traders");
    if (Array.isArray(traders) && traders.length > 0) {
      return traders[0]?.trader_id || fallbackId;
    }
    if (traders && Array.isArray(traders.traders) && traders.traders.length > 0) {
      return traders.traders[0]?.trader_id || fallbackId;
    }
  } catch {
    // ignore and fallback
  }
  return fallbackId;
}

// 处理活跃仓位
async function handleActivePositions(req: NextRequest) {
  const traderId = req.nextUrl.searchParams.get("trader_id") || await getDefaultTraderId();
  const url = `${API_BASE}/positions/active?trader_id=${traderId}`;
  
  const resp = await fetch(url, {
    cache: "no-store",
  });

  if (!resp.ok) {
    return NextResponse.json(
      { error: "Failed to fetch active positions" },
      { status: resp.status }
    );
  }

  const data = await resp.json();
  return NextResponse.json(data);
}

// 处理仓位历史
async function handlePositionHistory(req: NextRequest) {
  const traderId = req.nextUrl.searchParams.get("trader_id") || await getDefaultTraderId();
  const url = `${API_BASE}/positions/history?trader_id=${traderId}`;
  
  const resp = await fetch(url, {
    cache: "no-store",
  });

  if (!resp.ok) {
    return NextResponse.json(
      { error: "Failed to fetch position history" },
      { status: resp.status }
    );
  }

  const data = await resp.json();
  return NextResponse.json(data);
}

// 处理仓位详情
async function handlePositionDetail(req: NextRequest, positionId: string) {
  const traderId = req.nextUrl.searchParams.get("trader_id") || await getDefaultTraderId();
  const url = `${API_BASE}/positions/${positionId}?trader_id=${traderId}`;
  
  const resp = await fetch(url, {
    cache: "no-store",
  });

  if (!resp.ok) {
    return NextResponse.json(
      { error: "Failed to fetch position detail" },
      { status: resp.status }
    );
  }

  const data = await resp.json();
  return NextResponse.json(data);
}

async function handleLinkStatus(req: NextRequest) {
  const search = req.nextUrl.searchParams;
  const traderId = search.get("trader_id");
  const withTrader = (path: string) => {
    if (!traderId) return path;
    const suffix = `trader_id=${encodeURIComponent(traderId)}`;
    return path.includes("?") ? `${path}&${suffix}` : `${path}?${suffix}`;
  };

  const checks = [
    { key: "account", label: "账户概况", path: withTrader("/account") },
    { key: "status", label: "运行状态", path: withTrader("/status") },
    { key: "active-positions", label: "活跃仓位", path: withTrader("/positions/active") },
    { key: "exchange-trades", label: "交易所成交", path: withTrader("/exchange-trades?limit=5") },
  ];

  const items = await Promise.all(
    checks.map(async (check) => {
      try {
        await fetchJSON(check.path);
        return {
          key: check.key,
          label: check.label,
          linked: true,
          category: "data",
          endpoint: check.path,
        } as LinkStatusItemPayload;
      } catch (error) {
        return {
          key: check.key,
          label: check.label,
          linked: false,
          category: "data",
          endpoint: check.path,
          error: formatErrorMessage(error),
        } as LinkStatusItemPayload;
      }
    }),
  );
  const allLinked = items.every((item) => item.linked);

  return jsonResponse({
    all_linked: allLinked,
    items,
  });
}

async function forwardAction(req: NextRequest, actionPath: string) {
  const target = `${API_BASE}/${actionPath}${req.nextUrl.search}`;
  return proxyRequest(req, target);
}

async function proxyRequest(req: NextRequest, target: string) {
  const bodyText = await req.text();
  const hasBody = bodyText.length > 0;
  const contentType = req.headers.get("content-type") || "application/json";

  const upstream = await fetch(target, {
    method: "POST",
    cache: "no-store",
    headers: {
      Accept: "application/json",
      ...(hasBody ? { "content-type": contentType } : {}),
    },
    body: hasBody ? bodyText : undefined,
  });

  const data = await upstream.json().catch(() => ({}));
  return jsonResponse(data, { status: upstream.status });
}

export async function GET(req: NextRequest, ctx: { params: Promise<{ path: string[] }> }) {
  const { path } = await ctx.params;
  const parts = (path || []).filter(Boolean);
  const subpath = parts.join("/");

  try {
    if (parts[0] === "account-totals") {
      return await handleAccountTotals(req);
    }
    if (parts[0] === "trades") {
      return await handleTrades();
    }
    if (parts[0] === "exchange-trades") {
      return await handleExchangeTrades(req);
    }
    if (parts[0] === "positions" && parts[1] === "active") {
      return await handleActivePositions(req);
    }
    if (parts[0] === "positions" && parts[1] === "history") {
      return await handlePositionHistory(req);
    }
    if (parts[0] === "positions" && parts[1]) {
      return await handlePositionDetail(req, parts[1]);
    }
    if (parts[0] === "conversations") {
      return await handleConversations();
    }
    if (parts[0] === "status") {
      return await handleStatus(req);
    }
    if (parts[0] === "statistics") {
      return await handleStatistics(req);
    }
    if (parts[0] === "traders") {
      return await handleTraders();
    }
    if (parts[0] === "decisions" && parts[1] === "latest") {
      return await handleLatestDecisions(req);
    }
    if (parts[0] === "performance") {
      return await handlePerformance(req);
    }
    if (parts[0] === "link-status") {
      return await handleLinkStatus(req);
    }
  } catch (error) {
    console.error(`Aggregate handler failed for ${subpath}`, error);
    return jsonResponse({ error: (error as Error).message ?? "internal error" }, { status: 500 });
  }

  // Fallback proxy behaviour for any other paths
  const target = `${API_BASE}/${subpath}${req.nextUrl.search}`;
  try {
  const upstream = await fetch(target, {
    cache: "no-store",
    headers: {
        Accept: "application/json",
    },
  });
    const data = await upstream.json().catch(() => ({}));
    return jsonResponse(data, { status: upstream.status });
  } catch (error) {
    console.error(`Proxy fetch failed for ${target}`, error);
    return jsonResponse({ error: 'failed to proxy request' }, { status: 502 });
  }
}

export async function POST(req: NextRequest, ctx: { params: Promise<{ path: string[] }> }) {
  const { path } = await ctx.params;
  const parts = (path || []).filter(Boolean);
  const subpath = parts.join("/");

  try {
    if (parts[0] === "diagnostics") {
      if (parts[1] === "sync-positions") {
        return await forwardAction(req, "diagnostics/sync-positions");
      }
      if (parts[1] === "refresh-coin-pool") {
        return await forwardAction(req, "diagnostics/refresh-coin-pool");
      }
      if (parts[1] === "export-decisions") {
        return await forwardAction(req, "diagnostics/export-decisions");
      }
    }
  } catch (error) {
    console.error(`POST handler failed for ${subpath}`, error);
    return jsonResponse({ error: (error as Error).message ?? "internal error" }, { status: 500 });
  }

  const target = `${API_BASE}/${subpath}${req.nextUrl.search}`;
  try {
    return await proxyRequest(req, target);
  } catch (error) {
    console.error(`Proxy POST failed for ${target}`, error);
    return jsonResponse({ error: "failed to proxy request" }, { status: 502 });
  }
}

export async function OPTIONS() {
  return new NextResponse(null, {
    headers: {
      ...DEFAULT_HEADERS,
      "access-control-allow-methods": "GET,POST,OPTIONS",
      "access-control-allow-headers": "*",
    },
  });
}

function normalizeTradeSide(side?: string): "long" | "short" {
  const k = (side || "long").toString().toLowerCase();
  if (k.includes("short")) return "short";
  return "long";
}

function toUnixSeconds(value?: string | number, fallbackIndex?: number): number {
  if (value == null) {
    // ensure unique sorting fallback
    return Math.floor((Date.now() - (fallbackIndex ?? 0)) / 1000);
  }
  if (typeof value === "number") {
    return value > 1e12 ? Math.floor(value / 1000) : Math.floor(value);
  }
  const parsed = Date.parse(value);
  if (!Number.isNaN(parsed)) {
    return Math.floor(parsed / 1000);
  }
  const num = Number(value);
  if (Number.isFinite(num)) {
    return num > 1e12 ? Math.floor(num / 1000) : Math.floor(num);
  }
  return Math.floor((Date.now() - (fallbackIndex ?? 0)) / 1000);
}

function numberOrNull(v: unknown): number | null {
  const n = typeof v === "string" ? Number(v) : (v as number);
  if (Number.isFinite(n)) return n;
  return null;
}

function formatErrorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  return String(err);
}

function normalizeSymbolStats(stats?: Record<string, SymbolPerformanceEntry | undefined>) {
  if (!stats) return [];
  const arr: any[] = [];
  for (const [symbol, entry] of Object.entries(stats)) {
    if (!entry) continue;
    const baseSymbol = entry.symbol ?? symbol;
    const normSymbol = baseSymbol ? String(baseSymbol).toUpperCase() : "";
    arr.push({
      symbol: normSymbol,
      total_trades: entry.total_trades ?? 0,
      winning_trades: entry.winning_trades ?? 0,
      losing_trades: entry.losing_trades ?? 0,
      win_rate: entry.win_rate ?? 0,
      total_pn_l: entry.total_pn_l ?? 0,
      avg_pn_l: entry.avg_pn_l ?? 0,
    });
  }
  arr.sort((a, b) => Number(b.total_pn_l || 0) - Number(a.total_pn_l || 0));
  return arr;
}
