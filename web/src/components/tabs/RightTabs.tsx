"use client";
import { useSearchParams } from "next/navigation";
import { PositionsPanel } from "@/components/tabs/PositionsPanel";
import TradesTable from "@/components/trades/TradesTable";
import ExchangeTradesTable from "@/components/trades/ExchangeTradesTable";
import AiLearningPanel from "@/components/analytics/AiLearningPanel";
import MacroNewsPanel from "@/components/tabs/MacroNewsPanel";
import ModelChatPanel from "@/components/chat/ModelChatPanel";
import { ActivePositionsTable } from "@/components/positions/ActivePositionsTable";

export default function RightTabs() {
  const search = useSearchParams();
  const tab = search.get("tab") || "positions";
  if (tab === "chat") return <ModelChatPanel />;
  if (tab === "trades") return <TradesTable />;
  if (tab === "exchange-trades") return <ExchangeTradesTable />;
  if (tab === "active-positions") return <ActivePositionsTable />;
  if (tab === "analytics" || tab === "ai-learning") return <AiLearningPanel />;
  if (tab === "news") return <MacroNewsPanel />;
  return <PositionsPanel />;
}
