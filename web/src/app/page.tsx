import PriceTicker from "@/components/layout/PriceTicker";
import AccountValueChart from "@/components/chart/AccountValueChart";
import { PositionsPanel } from "@/components/tabs/PositionsPanel";
import { Suspense } from "react";

export default function Home() {
  return (
    <main className="w-full terminal-scan flex flex-col h-[calc(100vh-var(--header-h))]">
      <PriceTicker />
      <section className="grid grid-cols-1 gap-3 p-3 overflow-hidden lg:grid-cols-3 lg:gap-3 lg:p-3 h-[calc(100vh-var(--header-h)-var(--ticker-h))]">
        <div className="lg:col-span-2 h-full min-w-0 flex flex-col">
          <AccountValueChart />
        </div>
        <div className="lg:col-span-1 h-full overflow-hidden flex flex-col">
          {/* 整个右侧内容都在一个滚动容器内 */}
          <div className="h-full overflow-y-auto pr-1 custom-scrollbar">
            <Suspense
              fallback={
                <div className="mb-2 text-xs text-zinc-500">加载标签…</div>
              }
            >
              <div className="mb-2 flex flex-col gap-2 text-xs">
                <LinkStatusIndicator />
                <div className="flex flex-wrap items-center gap-3">
                  <TabButton name="持仓" tabKey="positions" />
                  <TabButton name="活跃仓位" tabKey="active-positions" />
                  <TabButton name="模型对话" tabKey="chat" />
                  <TabButton name="AI交易记录" tabKey="trades" />
                  <TabButton name="交易所成交记录" tabKey="exchange-trades" />
                  <TabButton name="AI学习与反思" tabKey="ai-learning" />
                  <TabButton name="宏观新闻" tabKey="news" />
                </div>
              </div>
            </Suspense>
            <Suspense
              fallback={<div className="text-xs text-zinc-500">加载数据…</div>}
            >
              <RightTabs />
            </Suspense>
          </div>
        </div>
      </section>
    </main>
  );
}

// Client subcomponents in separate file to keep server component clean
import RightTabs from "@/components/tabs/RightTabs";
import TabButton from "@/components/tabs/TabButton";
import LinkStatusIndicator from "@/components/shared/LinkStatusIndicator";
// RightTabs and TabButton are client components
