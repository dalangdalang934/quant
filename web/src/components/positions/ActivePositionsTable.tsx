"use client";
import { useActivePositions, ActivePosition } from "@/lib/api/hooks/useActivePositions";
import { fmtUSD, fmtNumber, pnlClass, withSign } from "@/lib/utils/formatters";
import { useState } from "react";
import Modal from "@/components/ui/Modal";

export function ActivePositionsTable() {
  const { positions, isLoading } = useActivePositions();
  const [selectedPosition, setSelectedPosition] = useState<ActivePosition | null>(null);

  if (isLoading) {
    return (
      <div className="p-4 text-center">
        <div className="animate-pulse">加载活跃仓位中...</div>
      </div>
    );
  }

  if (positions.length === 0) {
    return (
      <div className="p-4 text-center text-muted">
        当前没有活跃仓位
      </div>
    );
  }

  return (
    <>
      <div className="overflow-x-auto">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b" style={{ borderColor: "var(--panel-border)" }}>
              <th className="text-left py-1.5 px-2 text-[11px]">币种</th>
              <th className="text-center py-1.5 px-2 text-[11px]">方向</th>
              <th className="text-right py-1.5 px-2 text-[11px]">开仓价</th>
              <th className="text-right py-1.5 px-2 text-[11px]">最新价</th>
              <th className="text-right py-1.5 px-2 text-[11px]">数量</th>
              <th className="text-right py-1.5 px-2 text-[11px]">杠杆</th>
              <th className="text-right py-1.5 px-2 text-[11px]">未实现盈亏</th>
              <th className="text-center py-1.5 px-2 text-[11px]">状态</th>
              <th className="text-center py-1.5 px-2 text-[11px]">操作</th>
            </tr>
          </thead>
          <tbody>
            {positions.map((pos) => {
              return (
                <tr
                  key={pos.id}
                  className="border-b hover:bg-gray-50/5"
                  style={{ borderColor: "var(--panel-border)" }}
                >
                  <td className="py-1.5 px-2 font-semibold">{pos.symbol}</td>
                  <td className="text-center py-1.5 px-2">
                    <span className={`px-1.5 py-0.5 rounded text-[10px] ${
                      pos.side === "long" ? "bg-green-500/20 text-green-500" : "bg-red-500/20 text-red-500"
                    }`}>
                      {pos.side === "long" ? "做多" : "做空"}
                    </span>
                  </td>
                  <td className="text-right py-1.5 px-2">{fmtNumber(pos.open_price, 4)}</td>
                  <td className="text-right py-1.5 px-2">{fmtNumber(pos.mark_price, 4)}</td>
                  <td className="text-right py-1.5 px-2">{fmtNumber(pos.remaining_qty, 4)}</td>
                  <td className="text-right py-1.5 px-2">{pos.leverage}x</td>
                  <td className={`text-right py-1.5 px-2 ${pnlClass(pos.unrealized_pnl)}`}>
                    <div className="text-[11px]">{fmtUSD(pos.unrealized_pnl)}</div>
                    <div className="text-[10px] text-muted">{withSign(pos.unrealized_pnl_pct, 2)}%</div>
                  </td>
                  <td className="text-center py-1.5 px-2">
                    <span className={`px-1.5 py-0.5 rounded text-[10px] ${
                      pos.status === "open" ? "bg-green-500/20 text-green-500" :
                      pos.status === "partial_closed" ? "bg-yellow-500/20 text-yellow-500" :
                      "bg-gray-500/20 text-gray-500"
                    }`}>
                      {pos.status === "open" ? "持仓中" :
                       pos.status === "partial_closed" ? "部分平仓" : "已平仓"}
                    </span>
                  </td>
                  <td className="text-center py-1.5 px-2">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        setSelectedPosition(pos);
                      }}
                      className="text-blue-500 hover:text-blue-600 text-[10px] underline"
                    >
                      详情
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* 持仓详情弹窗 */}
      <Modal
        open={selectedPosition !== null}
        onClose={() => setSelectedPosition(null)}
        title={selectedPosition ? `${selectedPosition.symbol} 持仓详情` : "持仓详情"}
      >
        {selectedPosition && (
          <div className="space-y-3">
            {/* 基本信息 */}
            <div className="grid grid-cols-2 gap-3 text-xs">
              <div>
                <div className="text-[11px] text-muted mb-1">币种</div>
                <div className="font-semibold">{selectedPosition.symbol}</div>
              </div>
              <div>
                <div className="text-[11px] text-muted mb-1">方向</div>
                <div>
                  <span className={`px-2 py-0.5 rounded text-[10px] ${
                    selectedPosition.side === "long" ? "bg-green-500/20 text-green-500" : "bg-red-500/20 text-red-500"
                  }`}>
                    {selectedPosition.side === "long" ? "做多" : "做空"}
                  </span>
                </div>
              </div>
            </div>

            {/* 价格信息 */}
            <div className="border-t pt-3" style={{ borderColor: "var(--panel-border)" }}>
              <div className="grid grid-cols-2 gap-3 text-xs">
                <div>
                  <div className="text-[11px] text-muted mb-1">开仓价</div>
                  <div className="font-mono">{fmtNumber(selectedPosition.open_price, 4)}</div>
                </div>
                <div>
                  <div className="text-[11px] text-muted mb-1">最新价</div>
                  <div className="font-mono">{fmtNumber(selectedPosition.mark_price, 4)}</div>
                </div>
              </div>
            </div>

            {/* 持仓信息 */}
            <div className="border-t pt-3" style={{ borderColor: "var(--panel-border)" }}>
              <div className="grid grid-cols-2 gap-3 text-xs">
                <div>
                  <div className="text-[11px] text-muted mb-1">持仓数量</div>
                  <div className="font-mono">{fmtNumber(selectedPosition.remaining_qty, 4)}</div>
                </div>
                <div>
                  <div className="text-[11px] text-muted mb-1">杠杆倍数</div>
                  <div className="font-semibold">{selectedPosition.leverage}x</div>
                </div>
              </div>
            </div>

            {/* 盈亏信息 */}
            <div className="border-t pt-3" style={{ borderColor: "var(--panel-border)" }}>
              <div className="grid grid-cols-2 gap-3 text-xs">
                <div>
                  <div className="text-[11px] text-muted mb-1">未实现盈亏</div>
                  <div className={`font-semibold ${pnlClass(selectedPosition.unrealized_pnl)}`}>
                    {fmtUSD(selectedPosition.unrealized_pnl)}
                  </div>
                </div>
                <div>
                  <div className="text-[11px] text-muted mb-1">收益率</div>
                  <div className={`font-semibold ${pnlClass(selectedPosition.unrealized_pnl)}`}>
                    {withSign(selectedPosition.unrealized_pnl_pct, 2)}%
                  </div>
                </div>
              </div>
            </div>

            {/* 状态信息 */}
            <div className="border-t pt-3" style={{ borderColor: "var(--panel-border)" }}>
              <div className="grid grid-cols-2 gap-3 text-xs">
                <div>
                  <div className="text-[11px] text-muted mb-1">仓位状态</div>
                  <div>
                    <span className={`px-2 py-0.5 rounded text-[10px] ${
                      selectedPosition.status === "open" ? "bg-green-500/20 text-green-500" :
                      selectedPosition.status === "partial_closed" ? "bg-yellow-500/20 text-yellow-500" :
                      "bg-gray-500/20 text-gray-500"
                    }`}>
                      {selectedPosition.status === "open" ? "持仓中" :
                       selectedPosition.status === "partial_closed" ? "部分平仓" : "已平仓"}
                    </span>
                  </div>
                </div>
                <div>
                  <div className="text-[11px] text-muted mb-1">开仓时间</div>
                  <div className="text-[11px]">
                    {new Date(selectedPosition.open_time).toLocaleString('zh-CN', {
                      month: '2-digit',
                      day: '2-digit',
                      hour: '2-digit',
                      minute: '2-digit',
                      second: '2-digit'
                    })}
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}
      </Modal>
    </>
  );
}
