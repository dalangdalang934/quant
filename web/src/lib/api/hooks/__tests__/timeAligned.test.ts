import {
  getTimeUntilNextAlignment,
  getNextAlignedTimestamp,
  isNearAlignmentBoundary,
} from "../timeAligned";

describe("timeAligned", () => {
  describe("getTimeUntilNextAlignment", () => {
    it("should calculate time until next 10s boundary", () => {
      // Mock Date.now() to return 10:00:03.456 (3456ms after 10:00:00)
      const mockNow = 10_000 * 1000 + 3456; // 10000 seconds + 3.456 seconds
      jest.spyOn(Date, "now").mockReturnValue(mockNow);

      const result = getTimeUntilNextAlignment(10_000);

      // Next boundary is at 10:00:10.000, so wait time is 10000 - 3456 = 6544ms
      expect(result).toBe(6544);

      jest.restoreAllMocks();
    });

    it("should return full interval when exactly at boundary", () => {
      const mockNow = 10_000 * 1000; // Exactly at 10:00:00.000
      jest.spyOn(Date, "now").mockReturnValue(mockNow);

      const result = getTimeUntilNextAlignment(10_000);

      // Already at boundary, next is 10s away
      expect(result).toBe(10_000);

      jest.restoreAllMocks();
    });

    it("should handle different alignment intervals", () => {
      const mockNow = 15_000; // 15 seconds
      jest.spyOn(Date, "now").mockReturnValue(mockNow);

      // For 5s alignment: next boundary at 20s, wait 5s
      expect(getTimeUntilNextAlignment(5_000)).toBe(5_000);

      // For 30s alignment: next boundary at 30s, wait 15s
      expect(getTimeUntilNextAlignment(30_000)).toBe(15_000);

      jest.restoreAllMocks();
    });
  });

  describe("getNextAlignedTimestamp", () => {
    it("should return next aligned timestamp", () => {
      const mockNow = 10_000 * 1000 + 3456;
      jest.spyOn(Date, "now").mockReturnValue(mockNow);

      const result = getNextAlignedTimestamp(10_000);

      // Next boundary: 10:00:10.000
      expect(result).toBe(10_000 * 1000 + 10_000);

      jest.restoreAllMocks();
    });
  });

  describe("isNearAlignmentBoundary", () => {
    it("should detect when close to next boundary", () => {
      // 50ms before next boundary
      const mockNow = 10_000 * 1000 + 9950;
      jest.spyOn(Date, "now").mockReturnValue(mockNow);

      expect(isNearAlignmentBoundary(10_000, 100)).toBe(true);

      jest.restoreAllMocks();
    });

    it("should detect when close to previous boundary", () => {
      // 50ms after previous boundary
      const mockNow = 10_000 * 1000 + 50;
      jest.spyOn(Date, "now").mockReturnValue(mockNow);

      expect(isNearAlignmentBoundary(10_000, 100)).toBe(true);

      jest.restoreAllMocks();
    });

    it("should return false when not near boundary", () => {
      // 5s after boundary (middle of interval)
      const mockNow = 10_000 * 1000 + 5000;
      jest.spyOn(Date, "now").mockReturnValue(mockNow);

      expect(isNearAlignmentBoundary(10_000, 100)).toBe(false);

      jest.restoreAllMocks();
    });
  });
});
