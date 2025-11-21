"use client";

import { useEffect } from "react";
import { usePageActivityStore } from "@/store/usePageActivity";

function isDocumentActive() {
  if (typeof document === "undefined") return true;
  return document.visibilityState !== "hidden";
}

export default function PageActivityProvider() {
  const setActive = usePageActivityStore((s) => s.setActive);

  useEffect(() => {
    if (typeof document === "undefined") return;

    const handleVisibility = () => {
      setActive(isDocumentActive());
    };

    const handleFocus = () => setActive(true);
    const handleBlur = () => {
      if (!isDocumentActive()) setActive(false);
    };

    const handlePageHide = () => setActive(false);
    const handlePageShow = () => setActive(true);

    // Sync immediately on mount.
    handleVisibility();

    document.addEventListener("visibilitychange", handleVisibility);
    window.addEventListener("focus", handleFocus);
    window.addEventListener("blur", handleBlur);
    window.addEventListener("pageshow", handlePageShow);
    window.addEventListener("pagehide", handlePageHide);

    return () => {
      document.removeEventListener("visibilitychange", handleVisibility);
      window.removeEventListener("focus", handleFocus);
      window.removeEventListener("blur", handleBlur);
      window.removeEventListener("pageshow", handlePageShow);
      window.removeEventListener("pagehide", handlePageHide);
    };
  }, [setActive]);

  return null;
}
