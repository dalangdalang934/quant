#!/usr/bin/env tsx

/**
 * Test script to verify time-aligned polling behavior.
 *
 * This script simulates multiple clients requesting at different start times
 * and verifies they all converge to the same aligned boundaries.
 *
 * Run with: npx tsx scripts/test-time-alignment.ts
 */

function getTimeUntilNextAlignment(alignmentInterval: number): number {
  const now = Date.now();
  const timeSinceLastBoundary = now % alignmentInterval;
  const timeUntilNextBoundary = alignmentInterval - timeSinceLastBoundary;
  return timeUntilNextBoundary;
}

function simulateClient(clientId: number, startDelay: number, alignmentInterval: number) {
  setTimeout(() => {
    const now = Date.now();
    const delay = getTimeUntilNextAlignment(alignmentInterval);
    const nextBoundary = now + delay;
    const nextBoundaryDate = new Date(nextBoundary);

    console.log(
      `[Client ${clientId}] Started at ${new Date(now).toISOString()}, ` +
      `next request in ${delay}ms at ${nextBoundaryDate.toISOString()}`
    );

    // Schedule aligned request
    setTimeout(() => {
      const requestTime = Date.now();
      const expectedBoundary = Math.floor(requestTime / alignmentInterval) * alignmentInterval;
      const drift = requestTime - expectedBoundary;

      console.log(
        `[Client ${clientId}] ✓ Request sent at ${new Date(requestTime).toISOString()}, ` +
        `drift: ${drift}ms ${drift < 100 ? '(good)' : '(needs improvement)'}`
      );
    }, delay);
  }, startDelay);
}

async function main() {
  const alignmentInterval = 10_000; // 10 seconds

  console.log("=".repeat(80));
  console.log("Time-Aligned Polling Test");
  console.log("=".repeat(80));
  console.log(`Alignment interval: ${alignmentInterval}ms (${alignmentInterval / 1000}s)`);
  console.log(`Current time: ${new Date().toISOString()}`);
  console.log("\nSimulating 5 clients starting at random times...\n");

  // Simulate 5 clients starting at different times (within 3 seconds)
  for (let i = 1; i <= 5; i++) {
    const randomDelay = Math.floor(Math.random() * 3000); // Random delay 0-3s
    simulateClient(i, randomDelay, alignmentInterval);
  }

  // Wait for all clients to make their first request
  await new Promise(resolve => {
    const maxWait = alignmentInterval + 5000; // Wait up to alignment interval + 5s
    setTimeout(resolve, maxWait);
  });

  console.log("\n" + "=".repeat(80));
  console.log("Test completed!");
  console.log("All clients should have requested at the same 10s boundary.");
  console.log("=".repeat(80));
}

main().catch(console.error);
