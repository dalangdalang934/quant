"use client";

/**
 * Development-only debug utilities for time-aligned polling.
 *
 * Usage in browser console:
 * ```
 * window.__TIME_ALIGNMENT_DEBUG__ = true;
 * ```
 */

import { getTimeUntilNextAlignment, getNextAlignedTimestamp } from "./timeAligned";

export interface AlignmentDebugInfo {
  enabled: boolean;
  currentTime: string;
  nextBoundary: string;
  waitMs: number;
  alignmentInterval: number;
}

declare global {
  interface Window {
    __TIME_ALIGNMENT_DEBUG__?: boolean;
    __DEBUG_ALIGNMENT_INFO__?: () => AlignmentDebugInfo;
  }
}

let debugEnabled = false;
const requestLog: Array<{
  endpoint: string;
  timestamp: number;
  alignedBoundary: number;
  drift: number;
}> = [];

/**
 * Initialize debug mode if enabled via window.__TIME_ALIGNMENT_DEBUG__
 */
export function initDebugMode(): void {
  if (typeof window === "undefined" || process.env.NODE_ENV !== "development") {
    return;
  }

  debugEnabled = !!window.__TIME_ALIGNMENT_DEBUG__;

  // Expose debug info getter
  window.__DEBUG_ALIGNMENT_INFO__ = () => {
    const alignmentInterval = 10_000;
    const waitMs = getTimeUntilNextAlignment(alignmentInterval);
    const nextBoundary = getNextAlignedTimestamp(alignmentInterval);

    return {
      enabled: debugEnabled,
      currentTime: new Date().toISOString(),
      nextBoundary: new Date(nextBoundary).toISOString(),
      waitMs,
      alignmentInterval,
    };
  };

  if (debugEnabled) {
    console.log("[TimeAlignment] Debug mode enabled");
    console.log("[TimeAlignment] Next boundary:", new Date(getNextAlignedTimestamp(10_000)).toISOString());
  }
}

/**
 * Log a request for debugging (only if debug mode is enabled).
 */
export function logAlignedRequest(endpoint: string, alignmentInterval: number): void {
  if (!debugEnabled || typeof window === "undefined") {
    return;
  }

  const now = Date.now();
  const alignedBoundary = Math.floor(now / alignmentInterval) * alignmentInterval;
  const drift = now - alignedBoundary;

  requestLog.push({
    endpoint,
    timestamp: now,
    alignedBoundary,
    drift,
  });

  // Keep only last 100 requests
  if (requestLog.length > 100) {
    requestLog.shift();
  }

  // Log to console with color
  const driftMs = drift.toFixed(0);
  const style = drift < 100 ? "color: green" : drift < 500 ? "color: orange" : "color: red";

  console.log(
    `%c[TimeAlignment] ${endpoint} | Drift: ${driftMs}ms | Time: ${new Date(now).toISOString()}`,
    style,
  );
}

/**
 * Get statistics about request alignment quality.
 */
export function getAlignmentStats(): {
  totalRequests: number;
  avgDrift: number;
  maxDrift: number;
  wellAlignedPercent: number; // drift < 100ms
} {
  if (requestLog.length === 0) {
    return {
      totalRequests: 0,
      avgDrift: 0,
      maxDrift: 0,
      wellAlignedPercent: 0,
    };
  }

  const drifts = requestLog.map((r) => r.drift);
  const avgDrift = drifts.reduce((sum, d) => sum + d, 0) / drifts.length;
  const maxDrift = Math.max(...drifts);
  const wellAligned = drifts.filter((d) => d < 100).length;
  const wellAlignedPercent = (wellAligned / drifts.length) * 100;

  return {
    totalRequests: requestLog.length,
    avgDrift: Math.round(avgDrift),
    maxDrift: Math.round(maxDrift),
    wellAlignedPercent: Math.round(wellAlignedPercent),
  };
}

// Auto-initialize on module load (client-side only)
if (typeof window !== "undefined") {
  // Check for debug flag on next tick to allow user to set it
  setTimeout(initDebugMode, 0);
}
