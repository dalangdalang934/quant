"use client";
import { useParams } from "next/navigation";
import { useState, useEffect } from "react";
import { fmtUSD, fmtNumber } from "@/lib/utils/formatters";

interface Position {
  id: string;
  symbol: string;
  side: string;
  open_time: string;
  open_price: number;
  open_quantity: number;
  open_order_id: string;
  leverage: number;
  closes: Array<{
    close_time: string;
    close_price: number;
    close_qty: number;
    close_order_id: string;
    pnl: number;
    commission: number;
    reason: string;
  }>;
  status: string;
  remaining_qty: number;
  realized_pnl: number;
  total_commission: number;
  created_at: string;
  updated_at: string;
}

export default function PositionDetailPage() {
  const params = useParams();
  const positionId = params.id as string;
  const [position, setPosition] = useState<Position | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!positionId) return;

    fetch(`/api/nof1/positions/${positionId}`)
      .then((res) => {
        if (!res.ok) throw new Error("Failed to fetch position");
        return res.json();
      })
      .then((data) => {
        setPosition(data);
        setLoading(false);
      })
      .catch((err) => {
        setError(err.message);
        setLoading(false);
      });
  }, [positionId]);

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-pulse">加载中...</div>
      </div>
    );
  }

  if (error || !position) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-red-500">
          {error || "未找到仓位信息"}
        </div>
      </div>
    );
  }

  const totalCloseQty = position.closes.reduce((sum, c) => sum + c.close_qty, 0);
  const avgClosePrice = position.closes.length > 0
    ? position.closes.reduce((sum, c, _, arr) => sum + c.close_price / arr.length, 0)
    : 0;

  return (
    <div className="min-h-screen p-6">
      <div className="max-w-6xl mx-auto">
        <div className="mb-6">
          <h1 className="text-2xl font-bold mb-2">仓位详情</h1>
          <div className="text-sm text-muted">
            仓位ID: <span className="font-mono">{position.id}</span>
          </div>
        </div>

        {/* 基本信息卡片 */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mb-6">
          <div className="rounded-lg border p-6" style={{ background: "var(--panel-bg)", borderColor: "var(--panel-border)" }}>
            <h2 className="text-lg font-semibold mb-4">开仓信息</h2>
            <div className="space-y-2">
              <div className="flex justify-between">
                <span className="text-muted">币种</span>
                <span className="font-semibold">{position.symbol}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">方向</span>
                <span className={position.side === "long" ? "text-green-500" : "text-red-500"}>
                  {position.side === "long" ? "做多" : "做空"}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">开仓价格</span>
                <span>{fmtNumber(position.open_price, 4)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">开仓数量</span>
                <span>{fmtNumber(position.open_quantity, 4)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">杠杆倍数</span>
                <span>{position.leverage}x</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">开仓时间</span>
                <span className="text-xs">{new Date(position.open_time).toLocaleString()}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">订单ID</span>
                <span className="font-mono text-xs">{position.open_order_id}</span>
              </div>
            </div>
          </div>

          <div className="rounded-lg border p-6" style={{ background: "var(--panel-bg)", borderColor: "var(--panel-border)" }}>
            <h2 className="text-lg font-semibold mb-4">仓位状态</h2>
            <div className="space-y-2">
              <div className="flex justify-between">
                <span className="text-muted">状态</span>
                <span className={`rounded px-2 py-1 text-xs ${
                  position.status === "open" ? "bg-green-500/20 text-green-500" :
                  position.status === "partial_closed" ? "bg-yellow-500/20 text-yellow-500" :
                  "bg-gray-500/20 text-gray-500"
                }`}>
                  {position.status === "open" ? "持仓中" :
                   position.status === "partial_closed" ? "部分平仓" : "已平仓"}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">剩余数量</span>
                <span>{fmtNumber(position.remaining_qty, 4)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">已平仓数量</span>
                <span>{fmtNumber(totalCloseQty, 4)}</span>
              </div>
              {position.closes.length > 0 && (
                <div className="flex justify-between">
                  <span className="text-muted">平均平仓价</span>
                  <span>{fmtNumber(avgClosePrice, 4)}</span>
                </div>
              )}
              <div className="flex justify-between">
                <span className="text-muted">已实现盈亏</span>
                <span className={position.realized_pnl >= 0 ? "text-green-500" : "text-red-500"}>
                  {fmtUSD(position.realized_pnl)}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">总手续费</span>
                <span className="text-yellow-500">{fmtUSD(position.total_commission)}</span>
              </div>
            </div>
          </div>
        </div>

        {/* 平仓记录 */}
        {position.closes.length > 0 && (
          <div className="rounded-lg border p-6" style={{ background: "var(--panel-bg)", borderColor: "var(--panel-border)" }}>
            <h2 className="text-lg font-semibold mb-4">平仓记录</h2>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b" style={{ borderColor: "var(--panel-border)" }}>
                    <th className="text-left py-2">时间</th>
                    <th className="text-right py-2">平仓价</th>
                    <th className="text-right py-2">数量</th>
                    <th className="text-right py-2">盈亏</th>
                    <th className="text-right py-2">手续费</th>
                    <th className="text-left py-2">原因</th>
                    <th className="text-left py-2">订单ID</th>
                  </tr>
                </thead>
                <tbody>
                  {position.closes.map((close, idx) => (
                    <tr key={idx} className="border-b" style={{ borderColor: "var(--panel-border)" }}>
                      <td className="py-2 text-xs">{new Date(close.close_time).toLocaleString()}</td>
                      <td className="text-right py-2">{fmtNumber(close.close_price, 4)}</td>
                      <td className="text-right py-2">{fmtNumber(close.close_qty, 4)}</td>
                      <td className={`text-right py-2 ${close.pnl >= 0 ? "text-green-500" : "text-red-500"}`}>
                        {fmtUSD(close.pnl)}
                      </td>
                      <td className="text-right py-2 text-yellow-500">{fmtUSD(close.commission)}</td>
                      <td className="py-2 text-xs">{close.reason || "-"}</td>
                      <td className="py-2 font-mono text-xs">{close.close_order_id}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
