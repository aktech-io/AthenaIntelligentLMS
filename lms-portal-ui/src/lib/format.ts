export const formatKES = (amount: number) =>
  `KES ${amount.toLocaleString("en-KE", { minimumFractionDigits: 0, maximumFractionDigits: 0 })}`;
export const formatKESFull = (amount: number) =>
  `KES ${amount.toLocaleString("en-KE", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;

/** Intl-based money formatter; falls back gracefully on unknown currency codes. */
export const formatMoney = (
  amount: number | string | null | undefined,
  currency = "KES",
  opts: { compact?: boolean; decimals?: number } = {}
): string => {
  const n = Number(amount);
  const value = Number.isFinite(n) ? n : 0;
  const decimals = opts.decimals ?? (opts.compact ? 1 : 0);
  try {
    return new Intl.NumberFormat("en-KE", {
      style: "currency",
      currency: currency || "KES",
      currencyDisplay: "code",
      notation: opts.compact ? "compact" : "standard",
      minimumFractionDigits: decimals,
      maximumFractionDigits: opts.compact ? 1 : decimals,
    }).format(value);
  } catch {
    return `${currency} ${value.toLocaleString("en-KE", { maximumFractionDigits: decimals })}`;
  }
};

export const formatDate = (iso?: string | null): string => {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
};

export const formatDateTime = (iso?: string | null): string => {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString("en-GB", { day: "2-digit", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" });
};
