"use client";
import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";
import { fetcher } from "../nof1";

export interface ActivePosition {
  id: string;
  symbol: string;
  side: string;
  open_time: string;
  open_price: number;
  open_quantity: number;
  open_order_id: string;
  leverage: number;
  mark_price: number;
  status: string;
  remaining_qty: number;
  realized_pnl: number;
  total_commission: number;
  unrealized_pnl: number;
  unrealized_pnl_pct: number;
  created_at: string;
  updated_at: string;
}

// 币安持仓API返回的格式
interface BinancePosition {
  symbol: string;
  side: string;
  entry_price: number;
  mark_price: number;
  quantity: number;
  leverage: number;
  unrealized_pnl: number;
  unrealized_pnl_pct: number;
  liquidation_price?: number;
  margin_used?: number;
  margin_type?: string;
  stop_loss?: number;
  take_profit?: number;
}

export function useActivePositions() {
  // 改为使用 /api/positions 端点（与"持仓"页面相同）
  const { data, error, isLoading } = useSWR<BinancePosition[]>(
    "/api/nof1/positions?trader_id=binance_deepseek",
    fetcher,
    {
      ...activityAwareRefresh(5_000), // 5秒刷新一次
    },
  );

  // 将币安格式转换为ActivePosition格式
  const positions: ActivePosition[] = (data ?? []).map((pos, index) => ({
    id: `${pos.symbol}_${pos.side}_${index}`,
    symbol: pos.symbol,
    side: pos.side,
    open_time: new Date().toISOString(), // 币安API不提供开仓时间
    open_price: pos.entry_price,
    open_quantity: pos.quantity,
    open_order_id: "",
    leverage: pos.leverage,
    mark_price: pos.mark_price,
    status: "open",
    remaining_qty: pos.quantity,
    realized_pnl: 0,
    total_commission: 0,
    unrealized_pnl: pos.unrealized_pnl,
    unrealized_pnl_pct: pos.unrealized_pnl_pct,
    margin_type: pos.margin_type ?? "isolated",
    margin_used: pos.margin_used,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  }));

  return {
    positions,
    isLoading,
    isError: !!error,
  };
}
