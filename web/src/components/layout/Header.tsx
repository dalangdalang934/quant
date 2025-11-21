"use client";

import { useTheme } from "@/store/useTheme";

export function Header() {
  const theme = useTheme((s) => s.theme);
  const setTheme = useTheme((s) => s.setTheme);
  const barCls = `sticky top-0 z-50 w-full border-b backdrop-blur`;

  return (
    <header
      className={barCls}
      style={{
        background: "var(--header-bg)",
        borderColor: "var(--header-border)",
      }}
    >
      <div
        className="ui-sans flex h-[var(--header-h)] w-full items-center justify-end px-3 text-xs"
        style={{ color: "var(--foreground)" }}
      >
        <div className="hidden sm:flex items-center gap-1 text-[11px]">
            <div
            className="flex overflow-hidden rounded border"
              style={{ borderColor: "var(--chip-border)" }}
            >
            {(["dark", "light", "system"] as const).map((t) => (
                <button
                  key={t}
                  title={t}
                className="px-2 py-1 capitalize chip-btn"
                  style={
                    theme === t
                      ? {
                          background: "var(--btn-active-bg)",
                          color: "var(--btn-active-fg)",
                        }
                      : { color: "var(--btn-inactive-fg)" }
                  }
                onClick={() => setTheme(t)}
                >
                  {t}
                </button>
              ))}
          </div>
        </div>
      </div>
    </header>
  );
}

export default Header;
