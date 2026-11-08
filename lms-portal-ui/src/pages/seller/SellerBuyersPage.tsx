import { useMemo } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Building2, Info } from "lucide-react";
import { invoiceService, isSellerProfileMissing, type Buyer, type Invoice } from "@/services/invoiceService";
import { formatMoney } from "@/lib/format";
import { cn } from "@/lib/utils";
import { LoadingState, ErrorState, EmptyState } from "@/pages/invoice-finance/components/States";
import { TierPill } from "@/pages/invoice-finance/components/InvoiceStatusPill";
import { SellerShell } from "./components/SellerShell";
import { SellerOnboardingPage } from "./SellerOnboardingPage";

const TIER_COPY: Record<string, string> = {
  A: "Top-rated buyer — best advance rates.",
  B: "Reliable payer.",
  C: "Fair — slightly lower advance.",
  D: "Needs manual review.",
  UNRATED: "Not yet rated — rated after their first confirmation.",
};

type MyBuyer = { key: string; name: string; email?: string; tier?: string; buyer?: Buyer; invoices: number; total: number; currency: string };

/**
 * "My buyers": derived from the seller's own invoices (buyerId/buyerSnapshot),
 * enriched with the tier from GET /buyers when that endpoint is readable by the
 * seller token (it's officer-gated, so a 401/403 is tolerated → tiers hidden).
 */
const SellerBuyersPage = () => {
  const mine = useQuery({
    queryKey: ["seller-invoices", { all: true }],
    queryFn: () => invoiceService.listMine({ page: 0, size: 200 }),
    retry: (n, e) => !isSellerProfileMissing(e) && n < 2,
  });
  const buyers = useQuery<Buyer[]>({ queryKey: ["seller-buyers"], queryFn: () => invoiceService.listBuyers(), retry: false, enabled: !!mine.data });

  const rows = useMemo<MyBuyer[]>(() => {
    const invs: Invoice[] = mine.data?.content ?? [];
    const byId = new Map<string, Buyer>((buyers.data ?? []).map((b) => [b.id, b]));
    const map = new Map<string, MyBuyer>();
    for (const inv of invs) {
      const b = inv.buyerId ? byId.get(inv.buyerId) : undefined;
      const key = inv.buyerId ?? (inv.buyerSnapshot?.email ?? inv.buyerSnapshot?.name ?? "").toLowerCase();
      if (!key) continue;
      const cur = map.get(key) ?? {
        key,
        name: b?.name ?? inv.buyerSnapshot?.name ?? "Unnamed buyer",
        email: inv.buyerSnapshot?.email,
        tier: b?.tier,
        buyer: b,
        invoices: 0,
        total: 0,
        currency: inv.currency,
      };
      cur.invoices += 1;
      cur.total += Number(inv.total) || 0;
      if (!cur.email && inv.buyerSnapshot?.email) cur.email = inv.buyerSnapshot.email;
      map.set(key, cur);
    }
    return Array.from(map.values()).sort((a, b) => b.total - a.total);
  }, [mine.data, buyers.data]);

  if (isSellerProfileMissing(mine.error)) return <SellerOnboardingPage onRetry={() => mine.refetch()} retrying={mine.isFetching} />;

  const tiersHidden = buyers.isError;

  return (
    <SellerShell title="My buyers" subtitle="Companies that pay your invoices. Ratings are set by the bank and affect your advance rate.">
      <div className="space-y-4 animate-fade-in">
        {mine.isLoading ? (
          <LoadingState label="Loading buyers…" />
        ) : mine.isError ? (
          <ErrorState error={mine.error} onRetry={() => mine.refetch()} />
        ) : rows.length === 0 ? (
          <EmptyState icon={Building2} title="No buyers yet" description="Buyers appear here once you've submitted an invoice." />
        ) : (
          <ul className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            {rows.map((b) => (
              <li key={b.key} data-nemo-card="" className="rounded-xl border bg-card p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="font-medium truncate">{b.name}</p>
                    {b.email && <p className="text-xs text-muted-foreground truncate">{b.email}</p>}
                  </div>
                  {b.tier && <TierPill tier={b.tier} />}
                </div>
                <div className="mt-3 grid grid-cols-2 gap-2 text-sm">
                  <div>
                    <p className="text-[11px] uppercase tracking-wider text-muted-foreground">Invoices</p>
                    <p className="font-mono tabular-nums font-semibold">{b.invoices}</p>
                  </div>
                  <div>
                    <p className="text-[11px] uppercase tracking-wider text-muted-foreground">Total value</p>
                    <p className="font-mono tabular-nums font-semibold">{formatMoney(b.total, b.currency)}</p>
                  </div>
                  {b.buyer?.onTimeRate != null && (
                    <div>
                      <p className="text-[11px] uppercase tracking-wider text-muted-foreground">Pays on time</p>
                      <p className="font-mono tabular-nums font-semibold">{Math.round(b.buyer.onTimeRate * (b.buyer.onTimeRate <= 1 ? 100 : 1))}%</p>
                    </div>
                  )}
                  {b.buyer?.confirmationsCount != null && (
                    <div>
                      <p className="text-[11px] uppercase tracking-wider text-muted-foreground">Confirmations</p>
                      <p className="font-mono tabular-nums font-semibold">{b.buyer.confirmationsCount}</p>
                    </div>
                  )}
                </div>
                {b.tier && <p className={cn("text-xs mt-3", b.tier === "D" ? "text-warning" : "text-muted-foreground")}>{TIER_COPY[b.tier] ?? ""}</p>}
                <Link to={`/invoices`} className="text-xs text-primary hover:underline mt-2 inline-block">
                  See invoices
                </Link>
              </li>
            ))}
          </ul>
        )}
        {tiersHidden && rows.length > 0 && (
          <p className="text-xs text-muted-foreground inline-flex items-start gap-1.5">
            <Info className="h-3.5 w-3.5 shrink-0 mt-0.5" /> Buyer ratings are managed by the bank and aren't visible from this account.
          </p>
        )}
      </div>
    </SellerShell>
  );
};

export default SellerBuyersPage;
