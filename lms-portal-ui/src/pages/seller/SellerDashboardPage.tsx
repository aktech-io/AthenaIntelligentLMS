import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Banknote, CalendarClock, Upload, ArrowRight, Wallet, Receipt } from "lucide-react";
import { Button } from "@/components/ui/button";
import { invoiceService, isSellerProfileMissing, type Invoice, type SellerSummary } from "@/services/invoiceService";
import { formatMoney, formatDate } from "@/lib/format";
import { useAuth } from "@/contexts/AuthContext";
import { cn } from "@/lib/utils";
import { SellerShell } from "./components/SellerShell";
import { SellerOnboardingPage } from "./SellerOnboardingPage";
import { LoadingState, ErrorState, EmptyState } from "@/pages/invoice-finance/components/States";
import { statusLabel, daysUntil } from "./components/sellerUtils";
import { SellerStatusPill } from "./components/SellerStatusPill";

function Tile({ label, value, sub, icon: Icon, tone }: { label: string; value: string; sub?: React.ReactNode; icon: React.ComponentType<{ className?: string }>; tone?: "coral" | "aqua" }) {
  return (
    <div className="stat-card">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="eyebrow">{label}</p>
          <p className="num-display mt-1 truncate text-2xl md:text-3xl">{value}</p>
          {sub && <div className="text-xs text-muted-foreground mt-1">{sub}</div>}
        </div>
        <div className={cn("h-10 w-10 rounded-lg flex items-center justify-center shrink-0", tone === "aqua" ? "bg-accent/10" : "bg-primary/10")}>
          <Icon className={cn("h-5 w-5", tone === "aqua" ? "text-accent" : "text-primary")} />
        </div>
      </div>
    </div>
  );
}

const SellerDashboardPage = () => {
  const { user } = useAuth();
  const summary = useQuery<SellerSummary>({ queryKey: ["seller-summary"], queryFn: () => invoiceService.mineSummary(), refetchInterval: 30_000, retry: (n, e) => !isSellerProfileMissing(e) && n < 2 });
  const recent = useQuery({ queryKey: ["seller-invoices", { recent: true }], queryFn: () => invoiceService.listMine({ page: 0, size: 5 }), refetchInterval: 15_000, retry: (n, e) => !isSellerProfileMissing(e) && n < 2 });

  if (isSellerProfileMissing(summary.error) || isSellerProfileMissing(recent.error)) {
    return <SellerOnboardingPage onRetry={() => { summary.refetch(); recent.refetch(); }} retrying={summary.isFetching} />;
  }

  const s = summary.data;
  const ccy = s?.currency ?? "KES";
  const rows: Invoice[] = recent.data?.content ?? [];
  const offersReady = s?.countsByStatus?.OFFERED ?? 0;
  const nextDays = daysUntil(s?.nextDue?.dueDate);
  const firstName = (user?.name ?? "").split(" ")[0];

  return (
    <SellerShell title={firstName && !/^\+?\d/.test(firstName) ? `Hi, ${firstName}` : "Your invoices"} subtitle="Cash position across every invoice you've financed.">
      <div className="space-y-5 animate-fade-in">
        {/* CTA */}
        <Link
          to="/upload"
          className="group block rounded-xl p-5 md:p-6 text-primary-foreground bg-gradient-to-br from-primary to-[hsl(var(--nemo-coral-deep))] shadow-glow hover:brightness-110 transition"
        >
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-[11px] uppercase tracking-[0.16em] font-semibold text-white/80">Get paid early</p>
              <p className="font-heading text-xl md:text-2xl font-semibold mt-1">Finance an invoice</p>
              <p className="text-sm text-white/85 mt-1">Snap or upload a PDF. Most sellers see an offer within minutes of buyer confirmation.</p>
            </div>
            <span className="h-12 w-12 rounded-full bg-white/15 flex items-center justify-center shrink-0 group-hover:translate-x-0.5 transition-transform">
              <Upload className="h-6 w-6" />
            </span>
          </div>
        </Link>

        {offersReady > 0 && (
          <Link to="/invoices?f=offer" className="flex items-center justify-between rounded-lg border border-primary/40 bg-primary/10 px-4 py-3 text-sm hover:bg-primary/15 transition-colors">
            <span>
              <span className="font-semibold text-primary">{offersReady} offer{offersReady > 1 ? "s" : ""} ready</span> — accept to receive funds.
            </span>
            <ArrowRight className="h-4 w-4 text-primary" />
          </Link>
        )}

        {summary.isLoading ? (
          <LoadingState label="Loading your summary…" />
        ) : summary.isError ? (
          <ErrorState error={summary.error} onRetry={() => summary.refetch()} />
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <Tile label="Outstanding" value={formatMoney(s?.outstanding ?? 0, ccy)} sub="Owed by buyers on funded invoices" icon={Wallet} />
            <Tile label="Funded to date" value={formatMoney(s?.fundedTotal ?? 0, ccy)} sub={`${s?.total ?? rows.length} invoice${(s?.total ?? rows.length) === 1 ? "" : "s"} submitted`} icon={Banknote} tone="aqua" />
            <Tile
              label="Next payment due"
              value={s?.nextDue ? formatMoney(s.nextDue.amount, ccy) : "—"}
              sub={
                s?.nextDue ? (
                  <Link to={`/invoices/${s.nextDue.invoiceId}`} className="hover:text-foreground">
                    {formatDate(s.nextDue.dueDate)}
                    {nextDays != null && <span className={cn("ml-1", nextDays < 0 ? "text-destructive" : nextDays <= 7 ? "text-warning" : "")}>({nextDays < 0 ? `${-nextDays}d late` : `in ${nextDays}d`})</span>}
                  </Link>
                ) : (
                  "Nothing outstanding"
                )
              }
              icon={CalendarClock}
              tone="aqua"
            />
          </div>
        )}

        {/* Recent */}
        <section data-nemo-card="" className="rounded-xl border bg-card">
          <div className="flex items-center justify-between px-4 md:px-5 pt-4 pb-2">
            <h2 className="font-heading text-base font-semibold">Recent invoices</h2>
            <Button asChild variant="ghost" size="sm" className="text-primary">
              <Link to="/invoices">
                All <ArrowRight className="h-4 w-4" />
              </Link>
            </Button>
          </div>
          {recent.isLoading ? (
            <LoadingState />
          ) : recent.isError ? (
            <ErrorState error={recent.error} onRetry={() => recent.refetch()} />
          ) : rows.length === 0 ? (
            <EmptyState icon={Receipt} title="No invoices yet" description="Upload your first invoice to get an advance." action={<Button asChild size="sm"><Link to="/upload">Finance an invoice</Link></Button>} />
          ) : (
            <ul className="divide-y">
              {rows.map((inv) => (
                <li key={inv.id}>
                  <Link to={`/invoices/${inv.id}`} className="flex items-center gap-3 px-4 md:px-5 py-3 hover:bg-muted/50 transition-colors">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium truncate">
                        {inv.buyerSnapshot?.name ?? "Buyer to confirm"}
                        {inv.invoiceNumber && <span className="text-muted-foreground font-normal"> · #{inv.invoiceNumber}</span>}
                      </p>
                      <p className="text-xs text-muted-foreground mt-0.5">
                        {statusLabel(inv.status)}
                        {inv.dueDate && <> · due {formatDate(inv.dueDate)}</>}
                      </p>
                    </div>
                    <div className="text-right shrink-0">
                      <p className="font-mono tabular-nums text-sm font-semibold">{formatMoney(inv.total, inv.currency)}</p>
                      <SellerStatusPill status={inv.status} className="mt-1" />
                    </div>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </SellerShell>
  );
};

export default SellerDashboardPage;
