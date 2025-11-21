"use client";
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { usePositions } from "@/lib/api/hooks/usePositions";
import { fmtUSD, pnlClass } from "@/lib/utils/formatters";
import clsx from "clsx";
import ErrorBanner from "@/components/ui/ErrorBanner";
import { SkeletonRow } from "@/components/ui/Skeleton";
import { useAccountTotals } from "@/lib/api/hooks/useAccountTotals";
import PositionsFilter from "@/components/positions/PositionsFilter";
import { useSearchParams } from "next/navigation";
import { getModelColor, getModelName } from "@/lib/model/meta";
import { ModelLogoChip } from "@/components/shared/ModelLogo";
import CoinIcon from "@/components/shared/CoinIcon";
import StrategyOverview from "@/components/overview/StrategyOverview";

function splitSymbol(symbol?: string | null): [string, string] {
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

function getBase(symbol?: string | null): string {
  return splitSymbol(symbol)[0];
}

function renderSymbolParts(symbol?: string | null) {
  const [base, quote] = splitSymbol(symbol);
  if (!base && !quote) return "—";
  return quote ? (
    <span>
      {base}
      <span className="ml-1 text-[11px] text-zinc-500">/{quote}</span>
    </span>
  ) : (
    base
  );
}

type SortKey =
  | "symbol"
  | "leverage"
  | "entry_price"
  | "current_price"
  | "unrealized_pnl"
  | "side";

export function PositionsPanel() {
  // remove theme branching; rely on CSS variables
  const { positionsByModel, isLoading, isError } = usePositions();
  const { data: totalsData } = useAccountTotals();
  const [sortKey, setSortKey] = useState<SortKey>("unrealized_pnl");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const search = useSearchParams();
  const qModel = (search.get("model") || "ALL").toLowerCase();
  const qSymbol = (search.get("symbol") || "ALL").toUpperCase();
  const qSide = (search.get("side") || "ALL").toUpperCase();

  if (isLoading)
    return (
      <div
        className={`rounded-md border p-4`}
        style={{
          background: "var(--panel-bg)",
          borderColor: "var(--panel-border)",
        }}
      >
        <div className={`mb-2 text-sm`} style={{ color: "var(--muted-text)" }}>
          加载持仓中…
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <tbody>
              <SkeletonRow cols={7} />
              <SkeletonRow cols={7} />
              <SkeletonRow cols={7} />
            </tbody>
          </table>
        </div>
      </div>
    );

  if (!positionsByModel.length) {
    return (
      <div className={`text-sm`} style={{ color: "var(--muted-text)" }}>
        暂无持仓。
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <StrategyOverview />
      <ErrorBanner
        message={isError ? "上游持仓接口暂时不可用，请稍后重试。" : undefined}
      />
      <PositionsFilter
        models={positionsByModel.map((m) => m.id)}
        symbols={Array.from(
          new Set(
            positionsByModel.flatMap((m) => Object.keys(m.positions || {})),
          ),
        )}
      />
      {positionsByModel
        .filter((m) =>
          qModel === "all" ? true : m.id.toLowerCase() === qModel,
        )
        .map((m) => {
          const positionsRaw = Object.values(m.positions || {});
          type PositionWithSide = (typeof positionsRaw)[number] & {
            side: "LONG" | "SHORT";
          };
          const positions = (() => {
            const arr: PositionWithSide[] = positionsRaw.map((p) => ({
              ...p,
              side: p.quantity > 0 ? ("LONG" as const) : ("SHORT" as const),
            }));
            const dir = sortDir === "asc" ? 1 : -1;
            return arr.sort((a, b) => {
              const av = sortKey === "side" ? a.side : a[sortKey];
              const bv = sortKey === "side" ? b.side : b[sortKey];
              if (av == null && bv == null) return 0;
              if (av == null) return 1;
              if (bv == null) return -1;
              if (typeof av === "string" && typeof bv === "string")
                return av.localeCompare(bv) * dir;
              return (Number(av) - Number(bv)) * dir;
            });
          })();
          const filtered = positions
            .filter((p: any) =>
              qSymbol === "ALL" ? true : p.symbol?.toUpperCase() === qSymbol,
            )
            .filter((p: any) =>
              qSide === "ALL"
                ? true
                : (p.quantity > 0 ? "LONG" : "SHORT") === qSide,
            );
          const totalUnreal = filtered.reduce(
            (acc, p) => acc + (p.unrealized_pnl || 0),
            0,
          );
          const sumMargin = filtered.reduce(
            (acc, p) => acc + (p.margin || 0),
            0,
          );
          const sumRisk = filtered.reduce(
            (acc, p) => acc + (p.risk_usd || 0),
            0,
          );
          const avgConf = filtered.length
            ? filtered.reduce((a, p) => a + (p.confidence || 0), 0) /
              filtered.length
            : 0;

          // Extract latest totals snapshot for this model
          let equity: number | undefined;
          let realizedPnL: number | undefined;
          const list: any[] =
            totalsData && (totalsData as any).accountTotals
              ? (totalsData as any).accountTotals
              : [];
          for (let i = list.length - 1; i >= 0; i--) {
            const row = list[i];
            if (row?.model_id === m.id || row?.id === m.id) {
              equity = row.dollar_equity ?? row.equity ?? row.account_value;
              realizedPnL = row.realized_pnl;
              break;
            }
          }
          const availableCash = equity != null ? equity - sumMargin : undefined;
          const color = getModelColor(m.id);
          const brandBg = `linear-gradient(0deg, ${color}10, var(--panel-bg))`;
          const brandBorder = `${color}55`;
          return (
            <div
              key={m.id}
              className={`rounded-md border p-3`}
              style={{
                background: brandBg as any,
                borderColor: brandBorder as any,
              }}
            >
              <div className="mb-2 flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <ModelLogoChip modelId={m.id} size="sm" />
                  <div
                    className={`ui-sans text-sm font-semibold`}
                    style={{ color: "var(--foreground)" }}
                  >
                    {getModelName(m.id)}
                  </div>
                </div>
                <div
                  className={`ui-sans text-[11px]`}
                  style={{ color: "var(--muted-text)" }}
                >
                  未实现盈亏合计：
                  <span
                    className={
                      totalUnreal >= 0 ? "text-green-400" : "text-red-400"
                    }
                  >
                    {fmtUSD(totalUnreal)}
                  </span>
                </div>
              </div>
              <div
                className={`ui-sans mb-2 grid grid-cols-2 gap-x-4 gap-y-1 text-[11px]`}
                style={{ color: "var(--muted-text)" }}
              >
                <div>
                  净值：<span className="tabular-nums">{fmtUSD(equity)}</span>
                </div>
                <div>
                  已实现盈亏：
                  <span className="tabular-nums">{fmtUSD(realizedPnL)}</span>
                </div>
                <div>
                  可用现金≈
                  <span className="tabular-nums">{fmtUSD(availableCash)}</span>
                </div>
                <div>
                  风险金额合计：
                  <span className="tabular-nums">{fmtUSD(sumRisk)}</span>
                </div>
                <div>
                  平均置信度：
                  <span className="tabular-nums">
                    {avgConf ? (avgConf * 100).toFixed(1) + "%" : "—"}
                  </span>
                </div>
                <div>
                  持仓数量：
                  <span className="tabular-nums font-bold text-green-400">{filtered.length}</span>
                  <span className="text-xs ml-2">
                    ({filtered.map(p => p.symbol).join(", ")})
                  </span>
                </div>
              </div>
              {/* 持仓详情容器（不再需要内部滚动） */}
              {filtered.length > 0 && (
                <div 
                  className="mt-3" 
                  style={{ 
                    border: "1px solid rgba(255, 255, 255, 0.1)",
                    borderRadius: "0.5rem",
                    padding: "0.75rem",
                    backgroundColor: "rgba(0, 0, 0, 0.2)"
                  }}
                >
                {filtered.map((pos, index) => {
                // 为每个持仓显示开仓详情
                const firstPos = pos as any;
                const isLong = firstPos.quantity > 0;
                const qtyAbs = Math.abs(firstPos.quantity);
                const qtyValue = firstPos.quantity; // 保留符号
                const notional = qtyAbs * (firstPos.current_price ?? 0);
                
                // 获取保证金模式（逐仓/全仓）
                const marginType = firstPos.margin_type ?? "isolated";
                const marginTypeText = marginType === "cross" ? "全仓" : "逐仓";
                
                // 损益两平价：根据未实现盈亏和持仓信息计算
                // 公式：对于做空，盈亏两平价 = 开仓价 - (未实现盈亏 / 数量)
                // 更准确的公式需要考虑手续费，但这里先用价格差来估算
                let breakeven: number | string;
                if (typeof firstPos.entry_price === "number" && qtyAbs > 0) {
                  // 方法1：如果未实现盈亏接近0，说明当前价格接近盈亏两平价
                  // 方法2：根据盈亏计算价格差
                  const unrealizedPnl = firstPos.unrealized_pnl ?? 0;
                  // 价格调整量 = 未实现盈亏 / 持仓数量
                  // 对于做空：如果当前亏损，说明价格涨了，盈亏两平价应该更高（在当前价基础上调整）
                  // 对于做多：如果当前亏损，说明价格跌了，盈亏两平价应该更低
                  if (isLong) {
                    // 做多：盈亏两平价 = 开仓价 - (盈亏/数量) * (1/数量)
                    // 简化：盈亏两平价 ≈ 开仓价 + (当前价 - 开仓价) * (1 - 手续费调整)
                    // 由于手续费通常是双向的，盈亏两平价会略高于或低于开仓价
                    // 这里用近似值：如果未实现盈亏为0，当前价就是盈亏两平价
                    if (Math.abs(unrealizedPnl) < 0.01) {
                      breakeven = firstPos.current_price ?? firstPos.entry_price;
                    } else {
                      // 粗略估算：盈亏两平价 = 开仓价 + 手续费调整
                      const feeAdjustment = firstPos.entry_price * 0.0005; // 约0.05%
                      breakeven = firstPos.entry_price + feeAdjustment;
                    }
                  } else {
                    // 做空：类似处理
                    if (Math.abs(unrealizedPnl) < 0.01) {
                      breakeven = firstPos.current_price ?? firstPos.entry_price;
                    } else {
                      // 做空：根据实际观察，盈亏两平价 = 开仓价 - 手续费调整
                      // 从图片数据：开仓价107,635.10，盈亏两平价107,581.28，差值约0.05%
                      const feeRate = 0.0005; // 约0.05%手续费
                      const feeAdjustment = firstPos.entry_price * feeRate;
                      breakeven = firstPos.entry_price - feeAdjustment;
                    }
                  }
                  if (typeof breakeven === "number") {
                    breakeven = Number(breakeven.toFixed(2));
                  }
                } else {
                  breakeven = firstPos.entry_price ?? "—";
                }
                const baseSymbol = getBase(firstPos.symbol);
                // 格式化数量，保持适当精度（对于小数量使用更多小数位）
                const qtyDigits = qtyAbs >= 1 ? 2 : qtyAbs >= 0.1 ? 3 : 4;
                const qtyFormatted = qtyValue.toFixed(qtyDigits);
                
                return (
                  <div
                    key={`position-detail-${index}`}
                    className={`ui-sans mb-4 border rounded p-3`}
                    style={{ 
                      color: "var(--muted-text)",
                      borderColor: "rgba(255, 255, 255, 0.1)",
                      backgroundColor: "rgba(255, 255, 255, 0.02)"
                    }}
                  >
                    <div className="flex justify-between items-center mb-2">
                      <span className="text-xs font-bold" style={{ color: "var(--foreground)" }}>
                        持仓 #{index + 1} - {firstPos.symbol}
                      </span>
                      <span className={`text-xs ${isLong ? "text-green-400" : "text-red-400"}`}>
                        {isLong ? "做多" : "做空"}
                      </span>
                    </div>
                    <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-[11px]">
                    <div>
                      数量：<span className="tabular-nums" style={{ color: isLong ? "#16a34a" : "#ef4444" }}>
                        {qtyFormatted} {baseSymbol || ""}
                      </span>
                    </div>
                    <div>
                      开仓价格：<span className="tabular-nums">{fmtUSD(firstPos.entry_price)}</span>
                    </div>
                    <div>
                      标记价格：<span className="tabular-nums">{fmtUSD(firstPos.current_price)}</span>
                    </div>
                    <div>
                      损益两平价：<span className="tabular-nums">{typeof breakeven === "number" ? fmtUSD(breakeven) : breakeven}</span>
                    </div>
                    <div>
                      强平价格：<span className="tabular-nums" style={{ color: "#f97316" }}>
                        {firstPos.liquidation_price ? fmtUSD(firstPos.liquidation_price) : "—"}
                      </span>
                    </div>
                    <div>
                      保证金模式：<span className="tabular-nums">{marginTypeText}</span>
                    </div>
                    <div>
                      保证金：<span className="tabular-nums">{fmtUSD(firstPos.margin)}</span>
                    </div>
                    {firstPos.exit_plan && (
                      <>
                        <div>
                          止盈：<span className="tabular-nums" style={{ color: "var(--foreground)" }}>
                            {firstPos.exit_plan.profit_target ? fmtUSD(firstPos.exit_plan.profit_target) : "—"}
                          </span>
                        </div>
                        <div>
                          止损：<span className="tabular-nums" style={{ color: "var(--foreground)" }}>
                            {firstPos.exit_plan.stop_loss ? fmtUSD(firstPos.exit_plan.stop_loss) : "—"}
                          </span>
                        </div>
                      </>
                    )}
                    </div>
                  </div>
                );
              })}
                {/* 表格部分也在滚动容器内 */}
                <div className="overflow-x-auto mt-3">
                  <table className="w-full text-left text-[12px] terminal-text">
                  <thead
                    className={clsx("sticky top-0 z-10 ui-sans")}
                    style={{
                      background: "var(--panel-bg)",
                      color: "var(--muted-text)",
                    }}
                  >
                    <tr
                      className={clsx("border-b")}
                      style={{ borderColor: "var(--panel-border)" }}
                    >
                      <th className="py-1.5 pr-3">方向</th>
                      <th className="py-1.5 pr-3">币种</th>
                      <th className="py-1.5 pr-3">杠杆</th>
                      <th className="py-1.5 pr-3">名义金额</th>
                      <th className="py-1.5 pr-3">退出计划</th>
                      <th className="py-1.5 pr-3">未实现盈亏</th>
                    </tr>
                  </thead>
                  <tbody style={{ color: "var(--foreground)" }}>
                    {filtered.map((p, i) => {
                      const isLong = p.quantity > 0;
                      const notional =
                        Math.abs(p.quantity) * (p.current_price ?? 0);
                      return (
                        <tr
                          key={i}
                          className={clsx("border-b")}
                          style={{
                            borderColor:
                              "color-mix(in oklab, var(--panel-border) 50%, transparent)",
                          }}
                        >
                          <td
                            className="py-1.5 pr-3"
                            style={{ color: isLong ? "#16a34a" : "#ef4444" }}
                          >
                            {isLong ? "做多" : "做空"}
                          </td>
                          <td className="py-1.5 pr-3">
                            <span className="inline-flex items-center gap-1 justify-center">
                              <CoinIcon symbol={getBase(p.symbol)} size={16} />
                              <span className="ui-sans">
                                {renderSymbolParts(p.symbol)}
                              </span>
                            </span>
                          </td>
                          <td className="py-1.5 pr-3">{p.leverage}x</td>
                          <td className="py-1.5 pr-3 tabular-nums">
                            {fmtUSD(notional)}
                          </td>
                          <td className="py-1.5 pr-3">
                            <ExitPlanPeek plan={p.exit_plan} />
                          </td>
                          <td
                            className={clsx(
                              "py-1.5 pr-3 tabular-nums",
                              pnlClass(p.unrealized_pnl),
                            )}
                          >
                            {fmtUSD(p.unrealized_pnl)}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
                </div>
                {/* 添加一个底部标记，确认内容结束 */}
                <div className="text-center py-2 mt-3 text-xs" style={{ color: "var(--muted-text)" }}>
                  —— 共 {filtered.length} 个持仓 ——
                </div>
              </div>
              )}
            </div>
          );
        })}
    </div>
  );
}

function ExitPlanPeek({ plan }: { plan?: any }) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);
  const btnRef = useRef<HTMLButtonElement | null>(null);
  const popRef = useRef<HTMLDivElement | null>(null);
  
  // 检查是否有退出计划数据
  const hasPlan = plan && (plan.profit_target || plan.stop_loss || plan.invalidation_condition);

  useEffect(() => {
    // 如果没有退出计划或未打开，不需要设置事件监听
    if (!hasPlan || !open) return;
    const place = () => {
      const b = btnRef.current?.getBoundingClientRect();
      if (!b) return;
      const margin = 8;
      const width = 320;
      let left = Math.min(window.innerWidth - width - margin, b.right - width);
      if (left < margin) left = Math.max(margin, b.left);
      const top = Math.max(
        margin,
        Math.min(window.innerHeight - 160, b.bottom + 6),
      );
      setPos({ top, left });
    };
    place();
    const onDoc = (e: MouseEvent) => {
      const tgt = e.target as Node;
      if (popRef.current && popRef.current.contains(tgt)) return;
      if (btnRef.current && btnRef.current.contains(tgt)) return;
      setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, hasPlan]);

  // 如果没有退出计划，直接返回占位符
  if (!hasPlan) {
    return <span style={{ color: "var(--muted-text)" }}>—</span>;
  }

  return (
    <>
      <button
        ref={btnRef}
        className="ui-sans rounded border px-2 py-0.5 text-[11px]"
        style={{
          borderColor: "var(--panel-border)",
          color: "var(--foreground)",
        }}
        onClick={() => setOpen((v) => !v)}
      >
        查看
      </button>
      {open &&
        pos &&
        typeof document !== "undefined" &&
        createPortal(
          <div
            ref={popRef}
            className="w-80 rounded border p-2 text-[12px] shadow-xl"
            style={{
              position: "fixed",
              top: pos.top,
              left: pos.left,
              zIndex: 9999,
              background: "var(--panel-bg)",
              borderColor: "var(--panel-border)",
              color: "var(--foreground)",
            }}
          >
            <div className="ui-sans mb-1 font-semibold">退出计划</div>
            <div className="terminal-text text-xs leading-relaxed">
              <div>
                目标价：
                <span className="tabular-nums">
                  {plan.profit_target ?? "—"}
                </span>
              </div>
              <div>
                止损价：
                <span className="tabular-nums">{plan.stop_loss ?? "—"}</span>
              </div>
              <div>失效条件：</div>
              <div className="whitespace-pre-wrap">
                {plan.invalidation_condition || "—"}
              </div>
            </div>
          </div>,
          document.body,
        )}
    </>
  ) as any;
}
