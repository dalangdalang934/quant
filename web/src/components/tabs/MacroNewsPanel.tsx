"use client";
import { useMacroNews } from "@/lib/api/hooks/useMacroNews";
import ErrorBanner from "@/components/ui/ErrorBanner";
import { fmtTs, stripHtmlTags } from "@/lib/utils/formatters";

export default function MacroNewsPanel() {
  const { items, isLoading, isError } = useMacroNews();

  return (
    <div className="space-y-4">
      <ErrorBanner
        message={isError ? "宏观新闻源暂不可用，请稍后重试。" : undefined}
      />
      {isLoading && !items.length ? (
        <div className="text-xs" style={{ color: "var(--muted-text)" }}>
          正在拉取宏观新闻…
        </div>
      ) : null}
      <div className="space-y-3">
        {items.map((item) => (
          <article
            key={item.id}
            className="rounded-lg border p-4"
            style={{
              background: "var(--panel-bg)",
              borderColor: "var(--panel-border)",
              color: "var(--foreground)",
            }}
          >
            {/* 标题和时间戳 */}
            <header className="mb-3 flex items-start justify-between gap-3">
              <h3
                className="flex-1 font-semibold leading-tight text-base"
                style={{ color: "var(--foreground)" }}
              >
                {stripHtmlTags(item.headline) || "—"}
              </h3>
              <time
                className="shrink-0 text-xs tabular-nums"
                style={{ color: "var(--muted-text)" }}
                dateTime={item.published_at ?? item.created_at ?? undefined}
              >
                {fmtTs(item.timestamp ?? undefined)}
              </time>
            </header>

            {/* 来源信息 */}
            {(item.source || item.url) && (
              <div
                className="mb-3 flex items-center gap-3 text-xs"
                style={{ color: "var(--muted-text)" }}
              >
                {item.source && <span>来源：{item.source}</span>}
                {item.url && (
                  <a
                    href={item.url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-blue-400 hover:underline"
                    style={{ color: "var(--brand-accent)" }}
                  >
                    查看原文
                  </a>
                )}
              </div>
            )}

            {/* 正文内容 */}
            {item.summary && (
              <p
                className="mb-3 text-sm leading-relaxed"
                style={{ color: "var(--foreground)" }}
              >
                {stripHtmlTags(item.summary)}
              </p>
            )}

            {/* AI观点 */}
            {item.reasoning && (
              <div className="mb-3">
                <div
                  className="mb-1 text-xs font-medium"
                  style={{ color: "var(--foreground)" }}
                >
                  AI 观点：
                </div>
                <p
                  className="text-xs leading-relaxed"
                  style={{ color: "var(--muted-text)" }}
                >
                  {stripHtmlTags(item.reasoning)}
                </p>
              </div>
            )}

            {/* 标签 */}
            <div className="flex flex-wrap items-center gap-2">
              {item.impact && (
                <span
                  className={`rounded-md px-2.5 py-1 text-xs font-medium ${
                    item.impact === "利好" || item.impact === "bullish"
                      ? "bg-green-500/20 text-green-400"
                      : item.impact === "利空" || item.impact === "bearish"
                      ? "bg-red-500/20 text-red-400"
                      : ""
                  }`}
                  style={
                    item.impact === "中性" || item.impact === "neutral"
                      ? {
                          background: "var(--logo-chip-bg)",
                          color: "var(--muted-text)",
                        }
                      : undefined
                  }
                >
                  {item.impact === "bullish" || item.impact === "利好"
                    ? "利好"
                    : item.impact === "bearish" || item.impact === "利空"
                    ? "利空"
                    : item.impact === "neutral" || item.impact === "中性"
                    ? "中性"
                    : item.impact}
                </span>
              )}
              {item.sentiment && (
                <span
                  className={`rounded-md px-2.5 py-1 text-xs font-medium ${
                    item.sentiment === "positive" || item.sentiment === "看多"
                      ? "bg-green-500/20 text-green-400"
                      : item.sentiment === "negative" || item.sentiment === "看空"
                      ? "bg-orange-500/20 text-orange-400"
                      : ""
                  }`}
                  style={
                    item.sentiment === "neutral" || item.sentiment === "中性"
                      ? {
                          background: "var(--logo-chip-bg)",
                          color: "var(--muted-text)",
                        }
                      : undefined
                  }
                >
                  {item.sentiment === "positive"
                    ? "看多"
                    : item.sentiment === "negative"
                    ? "看空"
                    : item.sentiment === "看多"
                    ? "看多"
                    : item.sentiment === "看空"
                    ? "看空"
                    : "中性"}
                </span>
              )}
              {item.confidence && (
                <span
                  className="rounded-md px-2.5 py-1 text-xs"
                  style={{
                    background: "var(--logo-chip-bg)",
                    color: "var(--muted-text)",
                  }}
                >
                  置信度：{item.confidence}%
                </span>
              )}
            </div>
          </article>
        ))}
        {!isLoading && !items.length ? (
          <div className="text-xs" style={{ color: "var(--muted-text)" }}>
            暂无新闻
          </div>
        ) : null}
      </div>
    </div>
  );
}
