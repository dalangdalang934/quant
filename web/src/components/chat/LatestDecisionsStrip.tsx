"use client";

import { useMemo } from "react";
import { format } from "date-fns";
import { useLatestDecisions } from "@/lib/api/hooks/useLatestDecisions";
import ErrorBanner from "@/components/ui/ErrorBanner";

function toDate(ts: number | null) {
  if (ts == null) return null;
  const d = new Date(ts);
  return Number.isNaN(d.getTime()) ? null : d;
}

export default function LatestDecisionsStrip({ modelId }: { modelId?: string }) {
  const { rows, isLoading, isError } = useLatestDecisions(modelId);

  const items = useMemo(() => {
    const list: {
      model_id: string;
      model_name?: string;
      ai_model?: string;
      summary: string;
      timestamp: number | null;
      actionLabel: string;
    }[] = [];
    for (const row of rows) {
      for (const record of row.records ?? []) {
        const firstAction = record.actions?.[0];
        const action = firstAction?.action || record.summary || "—";
        list.push({
          model_id: row.model_id,
          model_name: row.model_name,
          ai_model: row.ai_model,
          summary: record.summary,
          timestamp: record.timestamp,
          actionLabel: `${(firstAction?.action || "").toUpperCase()} ${
            firstAction?.symbol ? String(firstAction.symbol).toUpperCase() : ""
          }`.trim() || record.summary,
        });
      }
    }
    list.sort((a, b) => (b.timestamp ?? 0) - (a.timestamp ?? 0));
    return list.slice(0, 6);
  }, [rows]);

  if (!isLoading && !items.length && !isError) {
    return null;
  }

  return (
    <div className="space-y-2">
      <ErrorBanner message={isError ? "最新决策数据暂不可用。" : undefined} />
      {isLoading && !items.length ? (
        <div className="text-[11px]" style={{ color: "var(--muted-text)" }}>
          正在加载最新决策…
        </div>
      ) : null}
      {items.length ? (
        <div
          className="flex flex-wrap gap-2 overflow-x-auto"
          style={{ color: "var(--foreground)" }}
        >
          {items.map((item, idx) => {
            const date = toDate(item.timestamp);
            const timeLabel = date ? format(date, "MM/dd HH:mm") : "--";
            return (
              <span
                key={`${item.model_id}-${idx}-${item.timestamp}`}
                className="inline-flex flex-col gap-1 rounded border px-3 py-2 text-[11px]"
                style={{ borderColor: "var(--panel-border)", background: "var(--panel-bg)" }}
              >
                <span className="ui-sans text-[10px] uppercase tracking-[0.2em]" style={{ color: "var(--muted-text)" }}>
                  {timeLabel}
                </span>
                <span className="ui-sans text-[10px] font-semibold" style={{ color: "var(--muted-text)" }}>
                  {item.model_name || item.model_id}
                </span>
                <span className="ui-sans font-semibold">
                  {item.actionLabel || item.summary || "—"}
                </span>
                {item.summary && item.summary !== item.actionLabel ? (
                  <span className="text-[10px]" style={{ color: "var(--muted-text)" }}>
                    {item.summary}
                  </span>
                ) : null}
              </span>
            );
          })}
        </div>
      ) : null}
    </div>
  );
}
