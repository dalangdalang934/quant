"use client";

import { create } from "zustand";

type ActivityCallback = () => void;

const activeCallbacks = new Set<ActivityCallback>();

const dispatchActiveCallbacks = () => {
  for (const cb of Array.from(activeCallbacks)) {
    try {
      cb();
    } catch (err) {
      console.error("usePageActivity onActive callback failed", err);
    }
  }
};

const deferDispatch = () => {
  if (typeof queueMicrotask === "function") {
    queueMicrotask(dispatchActiveCallbacks);
  } else {
    setTimeout(dispatchActiveCallbacks, 0);
  }
};

export interface PageActivityState {
  isActive: boolean;
  idleSince: number | null;
  lastActiveAt: number;
  setActive: (isActive: boolean) => void;
  registerOnActive: (cb: ActivityCallback) => () => void;
}

const initialActive = () => {
  if (typeof document === "undefined") return true;
  return document.visibilityState !== "hidden";
};

export const usePageActivityStore = create<PageActivityState>((set) => ({
  isActive: initialActive(),
  idleSince: null,
  lastActiveAt: Date.now(),
  setActive: (next) => {
    set((state) => {
      if (state.isActive === next) return state;
      if (next) {
        deferDispatch();
        return {
          ...state,
          isActive: true,
          idleSince: null,
          lastActiveAt: Date.now(),
        };
      }
      return {
        ...state,
        isActive: false,
        idleSince: Date.now(),
      };
    });
  },
  registerOnActive: (cb) => {
    activeCallbacks.add(cb);
    return () => activeCallbacks.delete(cb);
  },
}));

export function onNextPageActive(cb: ActivityCallback) {
  activeCallbacks.add(cb);
  return () => activeCallbacks.delete(cb);
}

export function isPageCurrentlyActive() {
  return usePageActivityStore.getState().isActive;
}

export function usePageActivity() {
  const isActive = usePageActivityStore((s) => s.isActive);
  const idleSince = usePageActivityStore((s) => s.idleSince);
  const lastActiveAt = usePageActivityStore((s) => s.lastActiveAt);
  return { isActive, idleSince, lastActiveAt };
}
