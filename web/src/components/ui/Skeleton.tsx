export function SkeletonRow({ cols = 6, as = "tr" }: { cols?: number; as?: "tr" | "div" }) {
  if (as === "div") {
    return (
      <div className="flex gap-4 py-2">
        {Array.from({ length: cols }).map((_, i) => (
          <div key={i} className="h-3 w-24 animate-pulse rounded skeleton-bg" />
        ))}
      </div>
    );
  }
  return (
    <tr>
      {Array.from({ length: cols }).map((_, i) => (
        <td key={i} className="py-2 pr-4">
          <div className="h-3 w-24 animate-pulse rounded skeleton-bg" />
        </td>
      ))}
    </tr>
  );
}

export function SkeletonBlock({ className = "h-64" }: { className?: string }) {
  return (
    <div className={`animate-pulse rounded-md skeleton-bg ${className}`} />
  );
}
