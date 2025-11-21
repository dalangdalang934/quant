"use client";

/**
 * Time-aligned polling utilities to reduce Fast Data Transfer costs.
 *
 * Strategy:
 * - Align all client requests to clock boundaries (e.g., every 10 seconds: 10:00:00, 10:00:10, 10:00:20)
 * - First request hits origin, subsequent requests hit Vercel Edge cache
 * - Cost savings: 95%+ with 100 concurrent users (1 origin hit vs 100)
 *
 * Example:
 * - Current time: 10:00:03.456
 * - Alignment interval: 10s
 * - Next aligned time: 10:00:10.000
 * - Delay before first request: 6.544s
 */

export interface TimeAlignedOptions {
  /**
   * Alignment interval in milliseconds (e.g., 10_000 for 10s)
   */
  alignmentInterval: number;

  /**
   * Whether to enable time alignment (default: true)
   */
  enabled?: boolean;

  /**
   * Maximum jitter to add after alignment in milliseconds (default: 0)
   * Useful to spread requests if single-origin performance is a concern
   */
  maxJitter?: number;
}

/**
 * Calculate milliseconds until next aligned time boundary.
 *
 * @param alignmentInterval - Alignment interval in ms (e.g., 10_000)
 * @returns Milliseconds to wait until next boundary
 *
 * @example
 * // Current time: 10:00:03.456, alignment: 10s
 * getTimeUntilNextAlignment(10_000) // Returns ~6544 (6.544s until 10:00:10)
 */
export function getTimeUntilNextAlignment(alignmentInterval: number): number {
  const now = Date.now();
  const timeSinceLastBoundary = now % alignmentInterval;
  const timeUntilNextBoundary = alignmentInterval - timeSinceLastBoundary;

  return timeUntilNextBoundary;
}

/**
 * Calculate the next aligned timestamp.
 *
 * @param alignmentInterval - Alignment interval in ms
 * @returns Unix timestamp (ms) of next aligned boundary
 */
export function getNextAlignedTimestamp(alignmentInterval: number): number {
  const now = Date.now();
  const timeUntilNext = getTimeUntilNextAlignment(alignmentInterval);
  return now + timeUntilNext;
}

/**
 * Check if current time is close to an alignment boundary.
 *
 * @param alignmentInterval - Alignment interval in ms
 * @param threshold - Threshold in ms to consider "close" (default: 100ms)
 * @returns True if within threshold of a boundary
 */
export function isNearAlignmentBoundary(
  alignmentInterval: number,
  threshold: number = 100,
): boolean {
  const timeUntilNext = getTimeUntilNextAlignment(alignmentInterval);
  const timeSinceLast = alignmentInterval - timeUntilNext;

  return timeUntilNext <= threshold || timeSinceLast <= threshold;
}

/**
 * Generate a random jitter value.
 *
 * @param maxJitter - Maximum jitter in ms
 * @returns Random value between 0 and maxJitter
 */
function getRandomJitter(maxJitter: number): number {
  return Math.floor(Math.random() * maxJitter);
}

/**
 * Create a time-aligned SWR refresh interval function.
 *
 * This function ensures all clients request at the same clock boundaries,
 * maximizing Vercel Edge cache hit rates.
 *
 * @param options - Time alignment options
 * @returns Function compatible with SWR's refreshInterval
 *
 * @example
 * useSWR(key, fetcher, {
 *   refreshInterval: createTimeAlignedInterval({
 *     alignmentInterval: 10_000, // Align to 10s boundaries
 *   })
 * })
 */
export function createTimeAlignedInterval(
  options: TimeAlignedOptions,
): () => number {
  const {
    alignmentInterval,
    enabled = true,
    maxJitter = 0,
  } = options;

  if (!enabled) {
    return () => alignmentInterval;
  }

  let isFirstCall = true;

  return () => {
    const baseDelay = getTimeUntilNextAlignment(alignmentInterval);
    const jitter = maxJitter > 0 ? getRandomJitter(maxJitter) : 0;
    const totalDelay = baseDelay + jitter;

    // Log debug info on first call (in development mode)
    if (isFirstCall && process.env.NODE_ENV === "development") {
      isFirstCall = false;
      const nextBoundary = new Date(Date.now() + baseDelay);
      console.log(`[TimeAlign] First interval: ${totalDelay}ms, next boundary: ${nextBoundary.toISOString()}`);
    }

    return totalDelay;
  };
}

/**
 * Debug helper to log alignment status (only in development).
 */
export function debugAlignment(
  name: string,
  alignmentInterval: number,
): void {
  if (process.env.NODE_ENV !== "development") {
    return;
  }

  const now = Date.now();
  const timeUntilNext = getTimeUntilNextAlignment(alignmentInterval);
  const nextBoundary = new Date(now + timeUntilNext);

  console.log(`[TimeAlign:${name}]`, {
    currentTime: new Date(now).toISOString(),
    nextBoundary: nextBoundary.toISOString(),
    waitMs: timeUntilNext,
    alignmentInterval,
  });
}
