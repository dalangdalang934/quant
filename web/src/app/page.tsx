import { Suspense } from "react";
import PriceTicker from "@/components/layout/PriceTicker";
import AccountValueChart from "@/components/chart/AccountValueChart";
import { PositionsPanel } from "@/components/tabs/PositionsPanel";
import TradesTable from "@/components/trades/TradesTable";

export default function Home() {
  return (
    <main className="w-full terminal-scan flex flex-col h-[calc(100vh-var(--header-h))]">
      <PriceTicker />
      <section className="grid grid-cols-1 gap-3 p-3 overflow-hidden lg:grid-cols-3 lg:gap-3 lg:p-3 h-[calc(100vh-var(--header-h)-var(--ticker-h))]">
        <div className="lg:col-span-2 h-full min-w-0 flex flex-col">
          <AccountValueChart />
        </div>
        <div className="lg:col-span-1 h-full overflow-hidden flex flex-col gap-3">
          <div className="h-1/2 min-h-[260px] overflow-hidden rounded-xl border border-white/5 bg-black/40">
            <Suspense fallback={<div className="p-3 text-xs text-zinc-500">加载持仓…</div>}>
              <PositionsPanel />
            </Suspense>
          </div>
          <div className="flex-1 min-h-[260px] overflow-hidden rounded-xl border border-white/5 bg-black/40">
            <Suspense fallback={<div className="p-3 text-xs text-zinc-500">加载历史订单…</div>}>
              <TradesTable />
            </Suspense>
          </div>
        </div>
      </section>
    </main>
  );
}
