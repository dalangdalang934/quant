"use client";
import { useEffect, useMemo, useRef, useState } from "react";
import useSWR from "swr";

const DEFAULT_TICKER_PAIRS = ["BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "DOGEUSDT", "XRPUSDT"] as const;

type TickerPair = string;

const sanitizePairs = (rawPairs: string | undefined): TickerPair[] => {
  if (!rawPairs) {
    return [...DEFAULT_TICKER_PAIRS];
  }

  const candidates = rawPairs
    .split(",")
    .map((token) => token.trim().toUpperCase())
    .filter((token) => token.length > 0);

  // 只保留由大写字母和/或数字组成的交易对，方便过滤非法输入
  const validPairs = candidates.filter((token) => /^[A-Z0-9]+$/.test(token));

  return validPairs.length > 0 ? validPairs : [...DEFAULT_TICKER_PAIRS];
};

const TICKER_PAIRS = sanitizePairs(process.env.NEXT_PUBLIC_TICKER_PAIRS);

interface TickerItem {
  label: string;
  price: number | null;
}

const toLabel = (pair: TickerPair): string => {
  if (pair.toUpperCase().endsWith("USDT")) {
    return pair.slice(0, -4);
  }
  return pair;
};

async function fetchTickers(): Promise<TickerItem[]> {
  const res = await Promise.all(
    TICKER_PAIRS.map(async (pair) => {
      try {
        const response = await fetch(`https://api.binance.com/api/v3/ticker/price?symbol=${pair}`);
        if (!response.ok) throw new Error("ticker");
        const data = await response.json();
        return { label: toLabel(pair), price: Number(data.price ?? 0) } as TickerItem;
      } catch (error) {
        return { label: toLabel(pair), price: null } as TickerItem;
      }
    }),
  );
  return res;
}

export default function PriceTicker() {
  const { data } = useSWR("binance-tickers", fetchTickers, {
    refreshInterval: 8000,
    revalidateOnFocus: false,
  });

  const items = useMemo(
    () => data ?? TICKER_PAIRS.map((pair) => ({ label: toLabel(pair), price: null })),
    [data],
  );

  const wrapRef = useRef<HTMLDivElement | null>(null);
  const trackRef = useRef<HTMLDivElement | null>(null);
  const [loop, setLoop] = useState(false);

  useEffect(() => {
    const wrap = wrapRef.current;
    const track = trackRef.current;
    if (!wrap || !track) return;

    const check = () => {
      const need = track.scrollWidth > wrap.clientWidth + 12;
      setLoop(need);
    };

    check();
    const ro = new ResizeObserver(check);
    ro.observe(wrap);
    ro.observe(track);
    return () => ro.disconnect();
  }, [items]);

  const renderItems = (source: TickerItem[], seed = 0) => (
    source.map(({ label, price }, idx) => (
      <span key={`${seed}-${label}-${idx}`} className="ticker-entry tabular-nums">
        <span className="ticker-symbol">{label}</span>
        <span className="ticker-price">
          {price != null
            ? price.toLocaleString(undefined, {
                style: "currency",
                currency: "USD",
                minimumFractionDigits: 2,
                maximumFractionDigits: 2,
              })
            : "—"}
        </span>
      </span>
    ))
  );

  return (
    <div className="arena-price-ticker-bar">
      <div ref={wrapRef} className="ticker-wrap">
        {loop ? (
          <div className="ticker-loop">
            <div ref={trackRef} className="ticker-track">
              {renderItems(items, 0)}
              {renderItems(items, 1)}
            </div>
          </div>
        ) : (
          <div ref={trackRef} className="ticker-inline">
            {renderItems(items, 0)}
          </div>
        )}
      </div>
    </div>
  );
}
