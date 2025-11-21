"use client";

import { useMemo } from "react";
import { useStatus } from "@/lib/api/hooks/useStatus";
import { useStatistics } from "@/lib/api/hooks/useStatistics";
import { useAccountTotals } from "@/lib/api/hooks/useAccountTotals";
import { fmtUSD, fmtInt, fmtTs } from "@/lib/utils/formatters";
import { ModelLogoChip } from "@/components/shared/ModelLogo";
import ErrorBanner from "@/components/ui/ErrorBanner";

function minutesToReadable(minutes?: number) {
  if (minutes == null || Number.isNaN(minutes)) return "—";
  if (minutes >= 60) {
    const h = Math.floor(minutes / 60);
    const m = Math.floor(minutes % 60);
    return `${h}小时${m}分`;
  }
  return `${Math.floor(minutes)}分`;
}

function percentDisplay(value?: number | null) {
  if (value == null || Number.isNaN(value)) return "—";
  return `${value >= 0 ? "+" : ""}${value.toFixed(2)}%`;
}

function isoToReadable(value?: string | null) {
  if (!value) return "—";
  const ms = Date.parse(value);
  if (Number.isNaN(ms)) return value;
  return fmtTs(Math.floor(ms / 1000));
}

export default function StrategyOverview() {
  const { rows: statusRows, isLoading: statusLoading, isError: statusError } = useStatus();
  const { rows: statisticsRows, isError: statisticsError } = useStatistics();
  const { data: totalsData } = useAccountTotals();

  const statsById = useMemo(() => {
    const map = new Map<string, (typeof statisticsRows)[number]>();
    for (const row of statisticsRows) map.set(row.model_id, row);
    return map;
  }, [statisticsRows]);

  const latestTotals = useMemo(() => {
    const rows = totalsData?.accountTotals ?? [];
    const map = new Map<string, { equity?: number; pnl?: number; pnlPct?: number }>();
    for (const row of rows) {
      const id = String(row.model_id ?? row.id ?? "");
      if (!id) continue;
      const prev = map.get(id);
      const ts = Number(row.timestamp ?? 0);
      const prevTs = prev && (prev as any).timestamp;
      if (!prev || (typeof prevTs === "number" && ts >= prevTs)) {
        map.set(id, {
          equity: Number(row.dollar_equity ?? row.account_value ?? 0),
          pnl: Number((row as any).total_pnl ?? row.realized_pnl ?? 0),
          pnlPct: Number(row.return_pct ?? 0),
          timestamp: ts,
        } as any);
      }
    }
    return map;
  }, [totalsData]);

  const cards = useMemo(() => {
    return statusRows.map((row) => {
      const stat = statsById.get(row.model_id);
      const totals = latestTotals.get(row.model_id);
      return {
        id: row.model_id,
        modelName: row.model_name || row.model_id,
        aiModel: row.ai_model,
        exchange: row.exchange,
        aiProvider: row.ai_provider,
        isRunning: row.is_running,
        runtime: minutesToReadable(row.runtime_minutes),
        callCount: fmtInt(row.call_count ?? 0),
        scanInterval: row.scan_interval || "—",
        stopUntil: isoToReadable(row.stop_until),
        lastReset: isoToReadable(row.last_reset_time),
        totalEquity: fmtUSD(row.total_equity ?? totals?.equity),
        totalPnl: fmtUSD(row.total_pnl ?? totals?.pnl),
        totalPnlPct: percentDisplay(
          row.total_pnl_pct ?? (typeof totals?.pnlPct === "number" ? totals?.pnlPct : undefined),
        ),
        positionCount: fmtInt(row.position_count ?? 0),
        marginUsedPct:
          row.margin_used_pct != null && !Number.isNaN(row.margin_used_pct)
            ? `${row.margin_used_pct.toFixed(2)}%`
            : "—",
        stat,
      };
    });
  }, [statusRows, statsById, latestTotals]);

  const hasData = cards.length > 0;

  return (
    <div className="space-y-3">
      <ErrorBanner
        message={statusError || statisticsError ? "策略状态数据暂不可用。" : undefined}
      />
      {statusLoading && !hasData ? (
        <div className="text-xs" style={{ color: "var(--muted-text)" }}>
          正在加载策略状态…
        </div>
      ) : null}
      {hasData ? (
        <div className="space-y-3">
          {cards.map((card) => (
            <article
              key={card.id}
              className="rounded-md border p-3"
              style={{
                background: "var(--panel-bg)",
                borderColor: "var(--panel-border)",
                color: "var(--foreground)",
              }}
            >
              <header className="flex items-center justify-between gap-3">
                <div className="flex items-center gap-2">
                  <ModelLogoChip modelId={card.id} size="md" />
                  <div>
                    <div className="ui-sans text-sm font-semibold">{card.modelName}</div>
                    <div className="text-[11px]" style={{ color: "var(--muted-text)" }}>
                      {card.aiModel?.toUpperCase?.() ?? "AI"}
                      {card.exchange ? ` · ${card.exchange}` : ""}
                    </div>
                  </div>
                </div>
                <span
                  className="rounded border px-2 py-0.5 text-[11px] ui-sans"
                  style={{
                    borderColor: card.isRunning
                      ? "rgba(34,197,94,0.35)"
                      : "rgba(148,163,184,0.35)",
                    color: card.isRunning ? "#22c55e" : "var(--muted-text)",
                    background: card.isRunning
                      ? "rgba(34,197,94,0.12)"
                      : "rgba(148,163,184,0.08)",
                  }}
                >
                  {card.isRunning ? "运行中" : "已暂停"}
                </span>
              </header>

              <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-[11px]">
                <Field label="运行时长" value={card.runtime} />
                <Field label="调用次数" value={card.callCount} />
                <Field label="扫描周期" value={card.scanInterval} />
                <Field label="保证金使用" value={card.marginUsedPct} />
                <Field label="净值" value={card.totalEquity} />
                <Field label="总盈亏" value={`${card.totalPnl} (${card.totalPnlPct})`} />
                <Field label="持仓数量" value={card.positionCount} />
                <Field
                  label="成功/失败周期"
                  value={`${fmtInt(card.stat?.successful_cycles ?? 0)} / ${fmtInt(card.stat?.failed_cycles ?? 0)}`}
                />
                <Field label="总周期" value={fmtInt(card.stat?.total_cycles ?? 0)} />
                <Field
                  label="开/平仓次数"
                  value={`${fmtInt(card.stat?.total_open_positions ?? 0)} / ${fmtInt(card.stat?.total_close_positions ?? 0)}`}
                />
              </div>
            </article>
          ))}
        </div>
      ) : (
        <div className="text-xs" style={{ color: "var(--muted-text)" }}>
          暂无策略状态信息。
        </div>
      )}
    </div>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="ui-sans text-[10px] uppercase tracking-[0.2em]" style={{ color: "var(--muted-text)" }}>
        {label}
      </div>
      <div className="mt-1 text-[12px] tabular-nums" style={{ color: "var(--foreground)" }}>
        {value || "—"}
      </div>
    </div>
  );
}
