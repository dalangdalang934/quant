"use client";
import { useMemo } from "react";
import { useLearning, LearningSummary, LearningTrade, SymbolStat } from "@/lib/api/hooks/useLearning";
import { useSearchParams, useRouter, usePathname } from "next/navigation";
import ErrorBanner from "@/components/ui/ErrorBanner";
import { fmtUSD, fmtPct, fmtInt } from "@/lib/utils/formatters";
import CoinIcon from "@/components/shared/CoinIcon";
import { ModelLogoChip } from "@/components/shared/ModelLogo";
import clsx from "clsx";
import { useTraderRegistry } from "@/lib/api/hooks/useTraderRegistry";
import { getModelName } from "@/lib/model/meta";

export default function AiLearningPanel() {
  const { models, isLoading, isError } = useLearning();
  const { traders } = useTraderRegistry();
  const search = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const qModel = (search.get("model") || "ALL").toLowerCase();

  const sortedModels = useMemo(() => {
    const arr = [...models];
    arr.sort((a, b) => (b.totals.total_trades ?? 0) - (a.totals.total_trades ?? 0));
    return arr;
  }, [models]);

  const current = useMemo(() => {
    if (!sortedModels.length) return undefined;
    if (qModel === "all") return sortedModels[0];
    return sortedModels.find((m) => m.model_id.toLowerCase() === qModel) ?? sortedModels[0];
  }, [sortedModels, qModel]);

  const labelById = useMemo(() => {
    const map = new Map<string, string>();
    for (const t of traders) {
      const id = t.model_id;
      if (!id) continue;
      map.set(id.toLowerCase(), t.model_name || getModelName(id));
    }
    return map;
  }, [traders]);

  const modelOptions = useMemo(
    () =>
      sortedModels.map((m) => ({
        id: m.model_id,
        label: labelById.get(m.model_id.toLowerCase()) || getModelName(m.model_id),
      })),
    [sortedModels, labelById],
  );

  if (!models.length && !isLoading)
    return (
      <div className={`text-sm`} style={{ color: "var(--muted-text)" }}>
        暂无可用的 AI 学习数据。
      </div>
    );

  return (
    <div className="space-y-3">
      <ErrorBanner message={isError ? "AI 学习数据暂不可用，请稍后重试。" : undefined} />
      <div className="flex flex-wrap items-center gap-2 text-[12px]" style={{ color: "var(--muted-text)" }}>
        <span className="ui-sans font-semibold tracking-wide">模型：</span>
        <select
          className="rounded border px-2 py-1 text-xs"
          style={{
            borderColor: "var(--panel-border)",
            background: "var(--panel-bg)",
            color: "var(--foreground)",
          }}
          value={current?.model_id ?? ""}
          onChange={(e) => {
            const params = new URLSearchParams(search.toString());
            if (!e.target.value) params.delete("model");
            else params.set("model", e.target.value);
            router.replace(`${pathname}?${params.toString()}`);
          }}
        >
          {modelOptions.map((opt) => (
            <option key={opt.id} value={opt.id}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>

      {current ? <LearningDetail model={current} isLoading={isLoading} /> : null}
    </div>
  );
}

function LearningDetail({ model, isLoading }: { model: LearningSummary; isLoading: boolean }) {
  const totals = model.totals;
  const rows = model.symbol_stats ?? [];
  const trades = model.recent_trades ?? [];

  return (
    <div className="space-y-4">
      <header
        className={clsx("rounded-md border p-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between")}
        style={{
          background: "var(--panel-bg)",
          borderColor: "var(--panel-border)",
          color: "var(--foreground)",
        }}
      >
        <div className="flex items-center gap-3">
          <ModelLogoChip modelId={model.model_id} size="md" />
          <div>
            <div className="ui-sans text-lg font-semibold">AI 学习与反思</div>
            <div className="text-xs" style={{ color: "var(--muted-text)" }}>
              {model.model_name ?? model.model_id} · {model.ai_model?.toUpperCase?.() ?? "AI"}
            </div>
          </div>
        </div>
        <div className="grid gap-2 text-xs sm:text-sm">
          <div>
            总交易：<span className="tabular-nums">{fmtInt(totals.total_trades)}</span>
          </div>
          <div>
            胜/负：
            <span className="tabular-nums">
              {fmtInt(totals.winning_trades)}W / {fmtInt(totals.losing_trades)}L
            </span>
          </div>
        </div>
      </header>

      <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        <SummaryCard title="胜率" value={fmtPercentDisplay(totals.win_rate)} hint="赢面" />
        <SummaryCard title="盈亏比" value={fmtNumber(totals.profit_factor, 2)} hint="Profit Factor" />
        <SummaryCard title="夏普比率" value={fmtNumber(totals.sharpe_ratio, 2)} hint="风险调整收益" />
        <SummaryCard title="平均盈利" value={fmtUSD(totals.avg_win)} hint="Avg Win" />
        <SummaryCard title="平均亏损" value={fmtUSD(totals.avg_loss)} hint="Avg Loss" />
      </section>

      <section className="rounded-md border" style={{ borderColor: "var(--panel-border)" }}>
        <div
          className="ui-sans border-b px-3 py-2 text-xs"
          style={{ borderColor: "var(--panel-border)", color: "var(--muted-text)" }}
        >
          币种表现
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-[12px]">
            <thead className="ui-sans" style={{ color: "var(--muted-text)" }}>
              <tr className="border-b" style={{ borderColor: "var(--panel-border)" }}>
                <th className="py-1.5 pr-3 text-center font-semibold">币种</th>
                <th className="py-1.5 pr-3">总交易</th>
                <th className="py-1.5 pr-3">胜率</th>
                <th className="py-1.5 pr-3">总盈亏</th>
                <th className="py-1.5 pr-3">均值</th>
              </tr>
            </thead>
            <tbody style={{ color: "var(--foreground)" }}>
              {rows.length ? (
                rows.map((stat) => (
                  <SymbolRow key={stat.symbol} stat={stat} />
                ))
              ) : (
                <tr>
                  <td colSpan={5} className="py-3 text-xs" style={{ color: "var(--muted-text)" }}>
                    {isLoading ? "加载中…" : "暂无币种表现数据"}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>

      <section className="rounded-md border" style={{ borderColor: "var(--panel-border)" }}>
        <div
          className="ui-sans border-b px-3 py-2 text-xs"
          style={{ borderColor: "var(--panel-border)", color: "var(--muted-text)" }}
        >
          最近交易复盘
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-[12px]">
            <thead className="ui-sans" style={{ color: "var(--muted-text)" }}>
              <tr className="border-b" style={{ borderColor: "var(--panel-border)" }}>
                <th className="py-1.5 pr-3 text-center font-semibold">币种</th>
                <th className="py-1.5 pr-3">方向</th>
                <th className="py-1.5 pr-3">数量</th>
                <th className="py-1.5 pr-3">盈亏</th>
                <th className="py-1.5 pr-3">持仓时长</th>
              </tr>
            </thead>
            <tbody style={{ color: "var(--foreground)" }}>
              {trades.length ? (
                trades.map((trade, idx) => <TradeRow key={`${trade.symbol}-${idx}`} trade={trade} />)
              ) : (
                <tr>
                  <td colSpan={5} className="py-3 text-xs" style={{ color: "var(--muted-text)" }}>
                    {isLoading ? "加载中…" : "暂无交易复盘数据"}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

function SummaryCard({ title, value, hint }: { title: string; value: string; hint?: string }) {
  return (
    <div
      className="rounded-md border p-3"
      style={{
        background: "var(--panel-bg)",
        borderColor: "var(--panel-border)",
        color: "var(--foreground)",
      }}
    >
      <div className="ui-sans text-xs" style={{ color: "var(--muted-text)" }}>
        {title}
      </div>
      <div className="mt-1 text-lg font-semibold tabular-nums">{value}</div>
      {hint ? (
        <div className="text-[11px]" style={{ color: "var(--muted-text)" }}>
          {hint}
        </div>
      ) : null}
    </div>
  );
}

function SymbolRow({ stat }: { stat: SymbolStat }) {
  const winRateDisplay = fmtPercentDisplay(stat.win_rate);
  const [base, quote] = formatSymbolParts(stat.symbol);
  return (
    <tr
      className="border-b"
      style={{ borderColor: "color-mix(in oklab, var(--panel-border) 50%, transparent)" }}
    >
      <td className="py-1.5 pr-3">
        <span className="inline-flex w-full items-center justify-center gap-2 text-center">
          <CoinIcon symbol={base || stat.symbol} size={16} />
          <span className="ui-sans text-sm font-semibold">
            {base || "—"}
            {quote ? <span className="ml-1 text-[11px] text-zinc-500">/{quote}</span> : null}
          </span>
        </span>
      </td>
      <td className="py-1.5 pr-3 tabular-nums">{fmtInt(stat.total_trades)}</td>
      <td className="py-1.5 pr-3 tabular-nums">{winRateDisplay}</td>
      <td className="py-1.5 pr-3 tabular-nums">{fmtUSD(stat.total_pn_l)}</td>
      <td className="py-1.5 pr-3 tabular-nums">{fmtUSD(stat.avg_pn_l)}</td>
    </tr>
  );
}

function TradeRow({ trade }: { trade: LearningTrade }) {
  const side = (trade.side || "").toLowerCase();
  const sideLabel = side.includes("short") ? "做空" : side.includes("long") ? "做多" : trade.side ?? "";
  const [base, quote] = formatSymbolParts(trade.symbol || "");
  return (
    <tr
      className="border-b"
      style={{ borderColor: "color-mix(in oklab, var(--panel-border) 50%, transparent)" }}
    >
      <td className="py-1.5 pr-3 text-center">
        <span className="inline-flex w-full items-center justify-center gap-2 text-center">
          <CoinIcon symbol={base || String(trade.symbol || "")} size={16} />
          <span className="ui-sans text-sm font-semibold">
            {base || "—"}
            {quote ? <span className="ml-1 text-[11px] text-zinc-500">/{quote}</span> : null}
          </span>
        </span>
      </td>
      <td className="py-1.5 pr-3" style={{ color: side.includes("short") ? "#ef4444" : "#16a34a" }}>
        {sideLabel}
      </td>
      <td className="py-1.5 pr-3 tabular-nums">{trade.quantity != null ? trade.quantity.toFixed(4) : "—"}</td>
      <td className="py-1.5 pr-3 tabular-nums">{fmtUSD(trade.pn_l)}</td>
      <td className="py-1.5 pr-3 tabular-nums" style={{ color: "var(--muted-text)" }}>
        {trade.duration || "—"}
      </td>
    </tr>
  );
}

function fmtNumber(n?: number, digits = 2) {
  if (n == null || Number.isNaN(n)) return "—";
  return Number(n).toFixed(digits);
}

function fmtPercentDisplay(n?: number) {
  if (n == null || Number.isNaN(n)) return "—";
  if (Math.abs(n) > 1 && Math.abs(n) <= 100) {
    return `${n.toFixed(1)}%`;
  }
  return fmtPct(n / 100);
}

function formatSymbolParts(symbol?: string | null): [string, string] {
  const s = String(symbol || "").toUpperCase();
  if (!s) return ["", ""];
  const suffixes = ["USDT", "USDC", "USD", "BTC", "ETH", "PERP", "SOL", "BNB"];
  for (const suf of suffixes) {
    if (s.endsWith(suf) && s.length > suf.length) {
      return [s.slice(0, -suf.length), suf];
    }
  }
  return [s, ""];
}
