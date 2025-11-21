"use client";
import { useMemo } from "react";
import { useTrades } from "@/lib/api/hooks/useTrades";
import ErrorBanner from "@/components/ui/ErrorBanner";
import { SkeletonRow } from "@/components/ui/Skeleton";
import { TradeItem } from "@/components/trades/TradeItemRow";

export default function TradesTable() {
  const { trades, isLoading, isError } = useTrades();
  const rows = useMemo(() => {
    const sorted = [...trades].sort(
      (a, b) =>
        Number(b.exit_time || b.entry_time) -
        Number(a.exit_time || a.entry_time),
    );
    return sorted.slice(0, 100);
  }, [trades]);

  const hasRows = rows.length > 0;
  const showSkeleton = isLoading && !hasRows;

  return (
    <div
      className="rounded-md border terminal-text text-[13px] sm:text-xs leading-relaxed"
      style={{
        background: "var(--panel-bg)",
        borderColor: "var(--panel-border)",
      }}
    >
      <header
        className="flex items-center justify-between px-3 py-2 border-b"
        style={{ borderColor: "var(--panel-border)" }}
      >
        <div className="text-sm ui-sans font-semibold" style={{ color: "var(--foreground)" }}>
          历史订单（最近 100 笔）
        </div>
        <div className="text-xs" style={{ color: "var(--muted-text)" }}>
          {rows.length} 条
        </div>
      </header>

      <ErrorBanner
        message={isError ? "历史订单数据暂不可用，请稍后重试。" : undefined}
      />

      <div
        className="divide-y max-h-[70vh] overflow-y-auto custom-scrollbar"
        style={{ borderColor: "color-mix(in oklab, var(--panel-border) 50%, transparent)" }}
      >
        {showSkeleton ? (
          <div className="p-3 space-y-2">
            <SkeletonRow cols={1} as="div" />
            <SkeletonRow cols={1} as="div" />
            <SkeletonRow cols={1} as="div" />
          </div>
        ) : hasRows ? (
          rows.map((t) => <TradeItem key={`${t.model_id}-${t.entry_time}-${t.exit_time}`} t={t} />)
        ) : (
          <div className="p-3 text-xs" style={{ color: "var(--muted-text)" }}>
            暂无历史订单。
          </div>
        )}
      </div>
    </div>
  );
}
