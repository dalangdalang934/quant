"use client";
import { SWRConfig, type Middleware } from "swr";
import type { PropsWithChildren } from "react";
import { useMemo } from "react";
import { usePageActivity } from "@/store/usePageActivity";
import type { ActivityAwareConfig } from "@/lib/api/hooks/activityAware";

const activityAwarePolling: Middleware = (useSWRNext) => {
  return (key, fetcher, config) => {
    const { isActive } = usePageActivity();
    const extended = (config ?? {}) as typeof config & ActivityAwareConfig;

    // If activity tracking is disabled or no interval provided, pass through.
    if (!extended || extended.disableActivityTracking) {
      return useSWRNext(key, fetcher, config);
    }

    const refresh = extended.refreshInterval as
      | number
      | ((latestData?: unknown) => number)
      | undefined;

    if (refresh == null) {
      return useSWRNext(key, fetcher, config);
    }

    const hiddenInterval =
      extended.refreshIntervalWhenHidden != null
        ? extended.refreshIntervalWhenHidden
        : 0;

    let appliedRefresh: typeof refresh;

    if (typeof refresh === "number") {
      // Keep legacy numeric intervals working with activity awareness.
      if (refresh <= 0) return useSWRNext(key, fetcher, config);
      appliedRefresh = isActive ? refresh : hiddenInterval;
    } else if (typeof refresh === "function") {
      // Support time-aligned function intervals. When the page is hidden,
      // honor the configured hidden interval to throttle/disable polling.
      appliedRefresh = isActive ? refresh : hiddenInterval;
    } else {
      // If refresh is neither number nor function, pass through unchanged
      return useSWRNext(key, fetcher, config);
    }

    const scopedConfig =
      config && config.refreshInterval === appliedRefresh
        ? config
        : ({
            ...(config ?? {}),
            refreshInterval: appliedRefresh as typeof refresh,
          } as typeof config);

    return useSWRNext(key, fetcher, scopedConfig);
  };
};

export default function SWRProvider({ children }: PropsWithChildren) {
  const middlewares = useMemo(() => [activityAwarePolling], []);

  return (
    <SWRConfig
      value={{
        // Ensure views refresh immediately when users return to the tab.
        revalidateOnFocus: true,
        revalidateOnReconnect: true,
        // If cache is stale, revalidate on access to avoid “stuck” data.
        revalidateIfStale: true,
        // Dedup short bursts but keep well below hook intervals (10s).
        dedupingInterval: 2_000,
        // Don’t aggressively retry; our data is mostly periodic.
        shouldRetryOnError: false,
        refreshWhenHidden: false,
        focusThrottleInterval: 2_000,
        use: middlewares,
      }}
    >
      {children}
    </SWRConfig>
  );
}
