"use client";
import { useMemo, useState } from "react";
import clsx from "clsx";
import {
  useLinkStatus,
  type LinkStatusItem,
} from "@/lib/api/hooks/useLinkStatus";
import { useSWRConfig } from "swr";

export default function LinkStatusIndicator() {
  const { status, isLoading, isError } = useLinkStatus();
  const { mutate } = useSWRConfig();
  const [showDetails, setShowDetails] = useState(false);
  const [showOps, setShowOps] = useState(false);

  const allGood = status?.all_linked;
  const hasData = Boolean(status?.items?.length);
  const headline = isLoading
    ? "链路检测中…"
    : isError
      ? "链路检测失败"
      : allGood
        ? "前后端链路已全部接通"
        : "存在未联通的功能";

  const items = status?.items ?? [];

  return (
    <div className="space-y-2">
      <div
        className={clsx(
          "rounded-md border px-3 py-2 flex flex-wrap items-center gap-2 text-[11px] sm:text-xs",
        )}
        style={{
          borderColor: allGood ? "#16a34a" : "var(--panel-border)",
          background: "var(--panel-bg)",
          color: "var(--foreground)",
        }}
      >
        <span
          className={clsx(
            "inline-flex h-2.5 w-2.5 rounded-full",
            allGood
              ? "bg-emerald-500"
              : isLoading
                ? "bg-amber-400"
                : "bg-rose-500",
          )}
          aria-hidden
        />
        <span className="flex-1 min-w-[180px]">{headline}</span>
        <button
          type="button"
          className="rounded border px-2 py-1 text-[11px]"
          style={{ borderColor: "var(--panel-border)" }}
          onClick={() => setShowDetails((v) => !v)}
          disabled={!hasData}
        >
          {showDetails ? "收起链路详情" : "链路详情"}
        </button>
        <button
          type="button"
          className="rounded border px-2 py-1 text-[11px]"
          style={{ borderColor: "var(--panel-border)" }}
          onClick={() => setShowOps((v) => !v)}
        >
          {showOps ? "收起运维工具" : "运维工具"}
        </button>
      </div>

      {showDetails && hasData ? (
        <DetailPanel items={items} />
      ) : null}

      {showOps ? (
        <OpsActionPanel
          onActionComplete={() => mutate("/api/nof1/link-status")}
        />
      ) : null}
    </div>
  );
}

function DetailPanel({ items }: { items: LinkStatusItem[] }) {
  return (
    <div
      className="w-full rounded-md border bg-black/75 p-3 text-[11px] text-zinc-100 shadow-inner"
      style={{ borderColor: "var(--panel-border)" }}
    >
      <div className="mb-2 font-semibold">链路详情</div>
      <ul className="space-y-1.5">
        {items.map((item) => (
          <li
            key={item.key}
            className="flex flex-col gap-0.5 rounded border px-2 py-1"
            style={{ borderColor: "var(--panel-border)" }}
          >
            <div className="flex items-center justify-between">
              <span>{item.label}</span>
              <span
                className={clsx(
                  "text-[10px]",
                  item.linked ? "text-emerald-400" : "text-rose-400",
                )}
              >
                {item.linked ? "已接通" : "未接通"}
              </span>
            </div>
            {item.error ? (
              <p className="text-[10px] text-zinc-400">{item.error}</p>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}

type OpsAction = {
  key: string;
  label: string;
  path: string;
  description?: string;
};

function OpsActionPanel({ onActionComplete }: { onActionComplete?: () => void }) {
  const actions: OpsAction[] = useMemo(
    () => [
      {
        key: "sync-positions",
        label: "同步持仓",
        path: "/api/nof1/diagnostics/sync-positions",
        description: "重新匹配日志与交易所实际仓位",
      },
      {
        key: "refresh-coin-pool",
        label: "刷新币种池",
        path: "/api/nof1/diagnostics/refresh-coin-pool",
        description: "立即更新AI500/默认币种池",
      },
      {
        key: "export-decisions",
        label: "导出决策日志",
        path: "/api/nof1/diagnostics/export-decisions",
        description: "获取最近的AI原始决策记录",
      },
      {
        key: "trigger-learning",
        label: "手动刷新AI学习",
        path: "/api/nof1/control/trigger-learning",
        description: "强制重建学习与反思状态",
      },
    ],
    [],
  );

  const [statusMap, setStatusMap] = useState<Record<string, { state: "idle" | "running" | "success" | "error"; message?: string }>>({});

  const runAction = async (action: OpsAction) => {
    setStatusMap((prev) => ({
      ...prev,
      [action.key]: { state: "running" },
    }));
    try {
      const res = await fetch(action.path, { method: "POST" });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        throw new Error(data?.error || "执行失败");
      }
      setStatusMap((prev) => ({
        ...prev,
        [action.key]: {
          state: "success",
          message: data?.message || "执行成功",
        },
      }));
      onActionComplete?.();
    } catch (error) {
      setStatusMap((prev) => ({
        ...prev,
        [action.key]: {
          state: "error",
          message: error instanceof Error ? error.message : String(error),
        },
      }));
    }
  };

  return (
    <div
      className="rounded-md border px-3 py-2 text-[11px] sm:text-xs"
      style={{
        borderColor: "var(--panel-border)",
        background: "var(--panel-bg)",
        color: "var(--foreground)",
      }}
    >
      <div className="mb-2 font-semibold text-[11px] text-zinc-200">
        运维工具
      </div>
      <div className="space-y-2">
        {actions.map((action) => {
          const current = statusMap[action.key];
          const running = current?.state === "running";
          const color =
            current?.state === "success"
              ? "text-emerald-400"
              : current?.state === "error"
                ? "text-rose-400"
                : "text-zinc-400";
          return (
            <div key={action.key} className="space-y-1">
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  disabled={running}
                  onClick={() => runAction(action)}
                  className={clsx(
                    "rounded border px-2 py-1 text-[11px]",
                    running ? "opacity-60" : "hover:border-emerald-400",
                  )}
                  style={{ borderColor: "var(--panel-border)" }}
                >
                  {running ? "执行中…" : action.label}
                </button>
                <span className="text-[10px] text-zinc-400">
                  {action.description}
                </span>
              </div>
              {current?.message ? (
                <div className={clsx("text-[10px]", color)}>
                  {current.message}
                </div>
              ) : null}
            </div>
          );
        })}
      </div>
    </div>
  );
}
