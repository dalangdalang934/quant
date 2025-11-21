"use client";
import { useMemo } from "react";
import { useTrades } from "@/lib/api/hooks/useTrades";
import { useSearchParams, useRouter, usePathname } from "next/navigation";
import ErrorBanner from "@/components/ui/ErrorBanner";
import { SkeletonRow } from "@/components/ui/Skeleton";
import { getModelName } from "@/lib/model/meta";
import { TradeItem } from "@/components/trades/TradeItemRow";

export default function TradesTable() {
  const { trades, isLoading, isError } = useTrades();
  const search = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();

  const qModel = (search.get("model") || "ALL").toLowerCase();

  const all = useMemo(() => {
    const arr = [...trades];
    arr.sort(
      (a, b) =>
        Number(b.exit_time || b.entry_time) -
        Number(a.exit_time || a.entry_time),
    );
    // show last 100 by default to match screenshot
    return arr.slice(0, 100);
  }, [trades]);

  const rows = useMemo(() => {
    return all.filter((t) =>
      qModel === "all" ? true : (t.model_id || "").toLowerCase() === qModel,
    );
  }, [all, qModel]);

  const models = useMemo(() => {
    const ids = Array.from(new Set(trades.map((t) => t.model_id))).filter(
      Boolean,
    ) as string[];
    return ids.sort((a, b) => a.localeCompare(b));
  }, [trades]);

  return (
    <div
      className={`rounded-md border terminal-text text-[13px] sm:text-xs leading-relaxed`}
      style={{
        background: "var(--panel-bg)",
        borderColor: "var(--panel-border)",
      }}
    >
      {/* Header: filter + count */}
      <div
        className="flex items-center justify-between px-3 py-2 border-b"
        style={{ borderColor: "var(--panel-border)" }}
      >
        <div
          className="flex items-center gap-2 text-sm ui-sans"
          style={{ color: "var(--foreground)" }}
        >
          <span
            className="font-semibold"
            style={{ color: "var(--foreground)" }}
          >
            筛选：
          </span>
          <select
            className="rounded border px-2 py-1 text-xs"
            style={{
              background: "var(--panel-bg)",
              borderColor: "var(--panel-border)",
              color: "var(--foreground)",
            }}
            value={search.get("model") || "ALL"}
            onChange={(e) => setQuery("model", e.target.value)}
          >
            <option value="ALL">全部模型</option>
            {models.map((m) => (
              <option key={m} value={m}>
                {getModelName(m)}
              </option>
            ))}
          </select>
        </div>
        <div
          className="text-xs font-semibold tabular-nums ui-sans"
          style={{ color: "var(--muted-text)" }}
        >
          展示最近 100 笔成交
        </div>
      </div>

      <ErrorBanner
        message={isError ? "成交记录数据源暂时不可用，请稍后重试。" : undefined}
      />

      {/* List */}
      <div className="divide-y" style={{ borderColor: "color-mix(in oklab, var(--panel-border) 50%, transparent)" }}>
        {isLoading ? (
          <div className="p-3 space-y-2">
            <SkeletonRow cols={1} as="div" />
            <SkeletonRow cols={1} as="div" />
            <SkeletonRow cols={1} as="div" />
          </div>
        ) : rows.length ? (
          rows.map((t) => <TradeItem key={t.id} t={t} />)
        ) : (
          <div className="p-3 text-xs" style={{ color: "var(--muted-text)" }}>
            暂无数据
          </div>
        )}
      </div>
    </div>
  );

  function setQuery(k: string, v: string) {
    const params = new URLSearchParams(search.toString());
    if (v === "ALL") params.delete(k);
    else params.set(k, v);
    router.replace(`${pathname}?${params.toString()}`);
  }
}
