#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
ETH/BTC 对冲动量策略回测脚本
--------------------------------

将用户提供的 Backtrader 逻辑标准化，方便直接运行/复用。特点：
- 使用 ETH/USDT、BTC/USDT 与 ETH/BTC 三条行情构建多空对冲
- 以 21/55 均线金叉死叉作为入场触发，静态 3% 止损、6% 止盈
- 允许通过命令行参数快速调节周期、杠杆与样本区间

依赖：
    pip install backtrader ccxt pandas
"""

from __future__ import annotations

import argparse
import time
from dataclasses import dataclass
from datetime import datetime, timedelta
from typing import Tuple

import backtrader as bt
import ccxt
import pandas as pd


class DualAssetStrategy(bt.Strategy):
    """ETH 多头 + BTC 空头 / 反向组合策略，实现方式与用户脚本保持一致。"""

    params = dict(
        fast_period=21,
        slow_period=55,
        leverage=10,
        stop_loss=0.03,
        take_profit=0.06,
    )

    def __init__(self):
        self.eth = self.datas[0]
        self.btc = self.datas[1]
        self.eth_btc = self.datas[2]

        self.fast_ma = bt.indicators.SMA(self.eth_btc.close, period=self.p.fast_period)
        self.slow_ma = bt.indicators.SMA(self.eth_btc.close, period=self.p.slow_period)

        self.current_position = None  # "long" -> 做多ETH/做空BTC, "short" -> 做空ETH/做多BTC
        self.entry_value = 0.0
        self.entry_price = 0.0
        self.trades = []

    # ====== 核心逻辑 ====== #

    def next(self):
        if len(self.fast_ma) < 2 or len(self.slow_ma) < 2:
            return

        equity = self.broker.getvalue()
        available_cash = self.broker.getcash()

        nominal_position = equity * self.p.leverage / 2
        max_nominal_by_cash = max(available_cash, 0) * self.p.leverage
        position_value = max(0, min(nominal_position, max_nominal_by_cash))
        if position_value <= 0:
            return

        fast_above = self.fast_ma[0] > self.slow_ma[0]
        fast_below = self.fast_ma[0] < self.slow_ma[0]
        prev_fast_above = self.fast_ma[-1] > self.slow_ma[-1]
        prev_fast_below = self.fast_ma[-1] < self.slow_ma[-1]

        if fast_above and prev_fast_below and self.current_position != "long":
            self._enter_long(position_value)
        elif fast_below and prev_fast_above and self.current_position != "short":
            self._enter_short(position_value)

        self._check_exit_conditions()

    # ====== 辅助函数 ====== #

    def close_pair_positions(self):
        self.close(self.eth, exectype=bt.Order.Market)
        self.close(self.btc, exectype=bt.Order.Market)

    def _enter_long(self, position_value: float):
        if self.current_position:
            self.close_pair_positions()

        eth_size = position_value / self.eth.close[0]
        btc_size = position_value / self.btc.close[0]

        self.buy(data=self.eth, size=eth_size)
        self.sell(data=self.btc, size=btc_size)

        self.current_position = "long"
        self.entry_value = self.broker.getvalue()
        self.entry_price = self.eth_btc.close[0]

        self.trades.append(
            dict(
                date=self.datetime.date(0),
                type="LONG",
                entry_price=self.entry_price,
                trade_volume=position_value * 2,
            )
        )

    def _enter_short(self, position_value: float):
        if self.current_position:
            self.close_pair_positions()

        eth_size = position_value / self.eth.close[0]
        btc_size = position_value / self.btc.close[0]

        self.sell(data=self.eth, size=eth_size)
        self.buy(data=self.btc, size=btc_size)

        self.current_position = "short"
        self.entry_value = self.broker.getvalue()
        self.entry_price = self.eth_btc.close[0]

        self.trades.append(
            dict(
                date=self.datetime.date(0),
                type="SHORT",
                entry_price=self.entry_price,
                trade_volume=position_value * 2,
            )
        )

    def _check_exit_conditions(self):
        if self.current_position is None:
            return

        current_value = self.broker.getvalue()
        pnl_pct = (current_value - self.entry_value) / self.entry_value

        exit_reason = None
        if pnl_pct <= -self.p.stop_loss:
            exit_reason = "STOP_LOSS"
        elif pnl_pct >= self.p.take_profit:
            exit_reason = "TAKE_PROFIT"
        elif (
            (self.current_position == "long" and self.fast_ma[0] < self.slow_ma[0])
            or (self.current_position == "short" and self.fast_ma[0] > self.slow_ma[0])
        ):
            exit_reason = "TREND_SWITCH"

        if exit_reason:
            if self.trades:
                self.trades[-1].update(
                    exit_date=self.datetime.date(0),
                    exit_price=self.eth_btc.close[0],
                    pnl_pct=pnl_pct * 100,
                    exit_reason=exit_reason,
                )
            self.close_pair_positions()
            self.current_position = None


def fetch_data(symbol: str, timeframe: str = "30m", days: int = 365) -> pd.DataFrame:
    exchange = ccxt.binance()
    exchange.enableRateLimit = True
    since = int((datetime.utcnow() - timedelta(days=days)).timestamp() * 1000)

    all_ohlcv = []
    current_since = since

    while True:
        try:
            ohlcv = exchange.fetch_ohlcv(symbol, timeframe, since=current_since, limit=1000)
            if not ohlcv:
                break
            all_ohlcv.extend(ohlcv)
            last_timestamp = ohlcv[-1][0]
            if last_timestamp >= (datetime.utcnow().timestamp() - 3600) * 1000:
                break
            current_since = last_timestamp + 1
            time.sleep(0.1)
        except Exception as exc:  # pylint: disable=broad-except
            raise RuntimeError(f"获取 {symbol} 数据失败: {exc}") from exc

    if not all_ohlcv:
        raise ValueError(f"{symbol} 无可用K线")

    df = pd.DataFrame(
        all_ohlcv,
        columns=["timestamp", "open", "high", "low", "close", "volume"],
    )
    df["timestamp"] = pd.to_datetime(df["timestamp"], unit="ms", utc=True)
    return df.drop_duplicates("timestamp").sort_values("timestamp").reset_index(drop=True)


def prepare_feeds(days: int = 365, timeframe: str = "30m") -> Tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    eth_usdt = fetch_data("ETH/USDT", timeframe, days).set_index("timestamp")
    btc_usdt = fetch_data("BTC/USDT", timeframe, days).set_index("timestamp")
    eth_btc = fetch_data("ETH/BTC", timeframe, days).set_index("timestamp")

    common_index = eth_usdt.index.intersection(btc_usdt.index).intersection(eth_btc.index)
    eth_usdt = eth_usdt.loc[common_index].sort_index()
    btc_usdt = btc_usdt.loc[common_index].sort_index()
    eth_btc = eth_btc.loc[common_index].sort_index()

    return eth_usdt, btc_usdt, eth_btc


@dataclass
class BacktestResult:
    final_value: float
    total_return_pct: float
    sharpe: float
    max_drawdown: float
    profit_factor: float


def run_backtest(
    days: int = 365,
    timeframe: str = "30m",
    fast_period: int = 21,
    slow_period: int = 55,
    leverage: int = 10,
    stop_loss: float = 0.03,
    take_profit: float = 0.06,
    initial_cash: float = 100_000.0,
) -> BacktestResult:
    cerebro = bt.Cerebro()
    eth_usdt, btc_usdt, eth_btc = prepare_feeds(days, timeframe)

    cerebro.adddata(bt.feeds.PandasData(dataname=eth_usdt), name="ETHUSDT")
    cerebro.adddata(bt.feeds.PandasData(dataname=btc_usdt), name="BTCUSDT")
    cerebro.adddata(bt.feeds.PandasData(dataname=eth_btc), name="ETHBTC")

    cerebro.addstrategy(
        DualAssetStrategy,
        fast_period=fast_period,
        slow_period=slow_period,
        leverage=leverage,
        stop_loss=stop_loss,
        take_profit=take_profit,
    )

    cerebro.broker.setcash(initial_cash)
    cerebro.broker.setcommission(
        commission=0.001,
        margin=1.0 / leverage,
        leverage=leverage,
        mult=1.0,
    )
    cerebro.broker.set_shortcash(True)

    cerebro.addanalyzer(bt.analyzers.TradeAnalyzer, _name="trades")
    cerebro.addanalyzer(bt.analyzers.SharpeRatio, _name="sharpe")
    cerebro.addanalyzer(bt.analyzers.DrawDown, _name="drawdown")

    results = cerebro.run()
    strat = results[0]

    final_value = cerebro.broker.getvalue()
    total_return_pct = (final_value / initial_cash - 1) * 100
    trade_analysis = strat.analyzers.trades.get_analysis()
    sharpe = strat.analyzers.sharpe.get_analysis().get("sharperatio", 0) or 0
    drawdown = strat.analyzers.drawdown.get_analysis().get("max", {}).get("drawdown", 0) or 0
    gross_profit = trade_analysis.get("won", {}).get("pnl", {}).get("gross", 0) or 0
    gross_loss = trade_analysis.get("lost", {}).get("pnl", {}).get("gross", 0) or 0
    profit_factor = float("inf") if gross_loss >= 0 else gross_profit / abs(gross_loss)

    print("\n=== ETH/BTC 双资产策略回测 ===")
    print(f"样本区间: {days} 天 @ {timeframe}")
    print(f"参数: 快线 {fast_period}, 慢线 {slow_period}, 杠杆 {leverage}x, 止损 {stop_loss:.2%}, 止盈 {take_profit:.2%}")
    print(f"期初资金 : {initial_cash:,.2f} USDT")
    print(f"最终净值 : {final_value:,.2f} USDT")
    print(f"总收益率 : {total_return_pct:.2f}%")
    print(f"夏普比率 : {sharpe:.2f}")
    print(f"最大回撤 : {drawdown:.2f}%")
    if profit_factor == float("inf"):
        print("Profit Factor: ∞（无亏损交易）")
    else:
        print(f"Profit Factor: {profit_factor:.2f}")

    return BacktestResult(
        final_value=final_value,
        total_return_pct=total_return_pct,
        sharpe=sharpe,
        max_drawdown=drawdown,
        profit_factor=profit_factor if profit_factor != float("inf") else float("inf"),
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="ETH/BTC 双资产右侧策略回测")
    parser.add_argument("--days", type=int, default=365, help="回测天数（默认 365）")
    parser.add_argument("--timeframe", type=str, default="30m", help="K线周期，默认 30m")
    parser.add_argument("--fast", type=int, default=21, help="快线周期 (SMA)")
    parser.add_argument("--slow", type=int, default=55, help="慢线周期 (SMA)")
    parser.add_argument("--leverage", type=int, default=10, help="名义杠杆倍数")
    parser.add_argument("--stop", type=float, default=0.03, help="止损阈值，例如 0.03=3%%")
    parser.add_argument("--take", type=float, default=0.06, help="止盈阈值，例如 0.06=6%%")
    parser.add_argument("--cash", type=float, default=100_000.0, help="期初资金 (USDT)")
    return parser.parse_args()


def main():
    args = parse_args()
    if args.fast >= args.slow:
        raise ValueError("快线周期必须小于慢线周期，否则无法形成交叉信号")

    run_backtest(
        days=args.days,
        timeframe=args.timeframe,
        fast_period=args.fast,
        slow_period=args.slow,
        leverage=args.leverage,
        stop_loss=args.stop,
        take_profit=args.take,
        initial_cash=args.cash,
    )


if __name__ == "__main__":
    main()
