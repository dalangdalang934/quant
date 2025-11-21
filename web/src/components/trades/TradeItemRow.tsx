"use client";
import { fmtUSD, fmtPrice } from "@/lib/utils/formatters";
import { getModelName, getModelColor } from "@/lib/model/meta";
import { ModelLogoChip } from "@/components/shared/ModelLogo";
import type { TradeRow } from "@/lib/api/hooks/useTrades";

export function TradeItem({ t }: { t: TradeRow }) {
  const sideColor = t.side === "long" ? "#16a34a" : "#ef4444";
  const modelColor = getModelColor(t.model_id || "");
  const symbol = (t.symbol || "").toUpperCase();
  const qty = t.quantity;
  const absQty = Math.abs(qty ?? 0);
  const entry = t.entry_price;
  const exit = t.exit_price;
  const notionalIn = absQty * (entry ?? 0);
  const notionalOut = absQty * (exit ?? 0);
  const hold = humanHold(t.entry_time, t.exit_time);
  const when = humanTime(t.exit_time || t.entry_time);
  const openTimeLabel = humanTime(t.entry_time);
  const closeTimeLabel = humanTime(t.exit_time);

  return (
    <div className="px-3 py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div
            className="mb-1 terminal-text text-[13px] sm:text-xs leading-relaxed"
            style={{ color: "var(--foreground)" }}
          >
            <span className="mr-1 align-middle">
              <ModelLogoChip modelId={t.model_id} size="sm" />
            </span>
              <span className="mr-1 text-[11px]" style={{ color: "var(--muted-text)" }}>
                策略
              </span>
              <b style={{ color: modelColor }}>{getModelName(t.model_id)}</b>
              <span> 完成了一笔 </span>
            <b style={{ color: sideColor }}>{sideZh(t.side)}</b>
            <span> 交易，标的 </span>
            <span className="inline-flex items-center gap-1 font-semibold">
              <CoinIcon symbol={symbol} />
              <span>{symbol}</span>
            </span>
          </div>
        </div>
        <div
          className="text-xs whitespace-nowrap tabular-nums"
          style={{ color: "var(--muted-text)" }}
        >
          {when}
        </div>
      </div>

      <div
        className="mt-1 grid grid-cols-1 gap-0.5 text-[13px] sm:text-xs leading-relaxed sm:grid-cols-2"
        style={{ color: "var(--foreground)" }}
      >
        <div>开仓价：<span className="tabular-nums">${fmtPrice(entry)}</span></div>
        <div>平仓价：<span className="tabular-nums">${fmtPrice(exit)}</span></div>
        <div>
          开仓金额：<span className="tabular-nums">{fmtUSD(notionalIn)}</span>
        </div>
        <div>
          平仓金额：<span className="tabular-nums">{fmtUSD(notionalOut)}</span>
        </div>
        <div>开仓数量：<span className="tabular-nums">{fmtNumber(qty, 2)}</span></div>
        <div>持有时长：{hold}</div>
        <div className="sm:col-span-2">
          杠杆：
          <span className="tabular-nums">{fmtLeverage(t.leverage)}</span>
        </div>
      </div>

      {((t.entry_time && t.entry_time > 0) || (t.exit_time && t.exit_time > 0)) && (
        <div className="mt-2 flex items-center gap-2 flex-wrap">
          {t.entry_time && t.entry_time > 0 && (
            <span
              className="rounded-md px-2 py-0.5 text-[11px] font-medium tabular-nums"
              style={{
                background: "rgba(59, 130, 246, 0.2)",
                color: "#3b82f6",
              }}
            >
              开仓：{openTimeLabel}
            </span>
          )}
          {t.exit_time && t.exit_time > 0 && (
            <span
              className="rounded-md px-2 py-0.5 text-[11px] font-medium tabular-nums"
              style={{
                background: "rgba(168, 85, 247, 0.2)",
                color: "#a855f7",
              }}
            >
              平仓：{closeTimeLabel}
            </span>
          )}
        </div>
      )}

      <div className="mt-2 flex items-center gap-2 flex-wrap">
        <span
          className="ui-sans text-[12px] sm:text-sm"
          style={{ color: "var(--muted-text)" }}
        >
          净盈亏：
        </span>
        <span
          className="terminal-text tabular-nums text-[13px] sm:text-sm font-semibold"
          style={{ color: pnlColor(t.realized_net_pnl) }}
        >
          {fmtUSD(t.realized_net_pnl)}
          {typeof t.pnl_pct === "number" && Number.isFinite(t.pnl_pct) && t.pnl_pct !== 0 && (
            <span className="ml-1 text-[12px] font-medium">
              (
              {t.pnl_pct >= 0 ? "+" : ""}
              {Math.abs(t.pnl_pct).toFixed(2)}%
              )
            </span>
          )}
        </span>
        {typeof t.is_partial_close === "boolean" && (
          <span
            className="ml-auto rounded-md px-2 py-0.5 text-[11px] font-medium"
            style={{
              background: t.is_partial_close
                ? "rgba(251, 191, 36, 0.2)"
                : "rgba(34, 197, 94, 0.2)",
              color: t.is_partial_close ? "#fbbf24" : "#22c55e",
            }}
          >
            {t.is_partial_close ? "部分平仓" : "全部平仓"}
          </span>
        )}
        {t.close_note && (
          <span
            className="rounded-md px-2 py-0.5 text-[11px] font-medium"
            style={{
              background: "rgba(148, 163, 184, 0.25)",
              color: "#475569",
            }}
          >
            {t.close_note}
          </span>
        )}
        {t.position_id && (
          <span
            className="rounded-md px-2 py-0.5 text-[11px] font-mono cursor-pointer hover:opacity-80"
            style={{
              background: "rgba(99, 102, 241, 0.2)",
              color: "#6366f1",
            }}
            onClick={(e) => {
              e.stopPropagation();
              // 点击查看仓位详情
              window.open(`/position/${t.position_id}`, '_blank');
            }}
            title="点击查看仓位详情"
          >
            ID: {t.position_id.slice(0, 8)}
          </span>
        )}
      </div>
    </div>
  );
}

function CoinIcon({ symbol }: { symbol: string }) {
  const src = coinSrc(symbol);
  if (!src) return <span className="inline-block text-[12px]">{symbol}</span>;
  return (
    <span className="logo-chip logo-chip-md overflow-hidden">
      <img src={src} alt={symbol} width={16} height={16} />
    </span>
  );
}

function coinSrc(symbol: string): string | undefined {
  const k = symbol.toUpperCase();
  switch (k) {
    case "BTC":
      return "/coins/btc.svg";
    case "ETH":
      return "/coins/eth.svg";
    case "SOL":
      return "/coins/sol.svg";
    case "BNB":
      return "/coins/bnb.svg";
    case "DOGE":
      return "/coins/doge.svg";
    case "XRP":
      return "/coins/xrp.svg";
    default:
      return undefined;
  }
}

function pnlColor(n?: number | null) {
  if (n == null || Number.isNaN(n)) return "var(--muted-text)";
  return n > 0 ? "#22c55e" : n < 0 ? "#ef4444" : "var(--muted-text)";
}

function humanTime(sec?: number | null) {
  if (!sec) return "--";
  const d = new Date(sec > 1e12 ? sec : sec * 1000);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(d.getMonth() + 1)}/${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function humanHold(entry?: number | null, exit?: number | null) {
  if (!entry) return "—";
  const a = entry > 1e12 ? entry : entry * 1000;
  const b = exit ? (exit > 1e12 ? exit : exit * 1000) : Date.now();
  const ms = Math.max(0, b - a);
  const m = Math.floor(ms / 60000);
  const h = Math.floor(m / 60);
  const mm = m % 60;
  return h ? `${h}小时${mm}分` : `${mm}分`;
}


function fmtNumber(n?: number | null, digits = 2) {
  if (n == null || Number.isNaN(n)) return "--";
  const sign = n < 0 ? "-" : "";
  const v = Math.abs(n).toLocaleString(undefined, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  });
  return `${sign}${v}`;
}

function fmtLeverage(n?: number | null) {
  if (n == null || Number.isNaN(n) || n <= 0) return "--";
  return `${n}x`;
}

function sideZh(s?: string) {
  return s === "long" ? "做多" : s === "short" ? "做空" : String(s ?? "—");
}

