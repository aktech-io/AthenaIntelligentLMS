import { Link, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Receipt, Upload } from "lucide-react";
import { Button } from "@/components/ui/button";
import { invoiceService, isSellerProfileMissing, type Invoice, type InvoiceStatus } from "@/services/invoiceService";
import { formatMoney, formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import { LoadingState, ErrorState, EmptyState } from "@/pages/invoice-finance/components/States";
import { SellerShell } from "./components/SellerShell";
import { SellerOnboardingPage } from "./SellerOnboardingPage";
import { SellerStatusPill } from "./components/SellerStatusPill";
import { SELLER_FILTERS, statusHint, daysUntil } from "./components/sellerUtils";

const PAGE_SIZE = 20;

const SellerInvoicesPage = () => {
  const [params, setParams] = useSearchParams();
  const fkey = params.get("f") ?? "all";
  const filter = SELLER_FILTERS.find((f) => f.key === fkey) ?? SELLER_FILTERS[0];
  const page = Number(params.get("page") ?? 0) || 0;
  // Single-status groups filter server-side; multi-status groups fetch all and filter client-side.
  const serverStatus: InvoiceStatus | "" = filter.statuses.length === 1 ? filter.statuses[0] : "";

  const q = useQuery({
    queryKey: ["seller-invoices", { status: serverStatus, page, size: PAGE_SIZE }],
    queryFn: () => invoiceService.listMine({ status: serverStatus, page, size: PAGE_SIZE }),
    refetchInterval: 15_000,
    retry: (n, e) => !isSellerProfileMissing(e) && n < 2,
  });

  if (isSellerProfileMissing(q.error)) return <SellerOnboardingPage onRetry={() => q.refetch()} retrying={q.isFetching} />;

  const all: Invoice[] = q.data?.content ?? [];
  const rows = filter.statuses.length > 1 ? all.filter((i) => (filter.statuses as string[]).includes(i.status)) : all;
  const totalPages = q.data?.totalPages ?? 1;

  const setFilter = (key: string) => {
    const p = new URLSearchParams(params);
    if (key === "all") p.delete("f");
    else p.set("f", key);
    p.delete("page");
    setParams(p, { replace: true });
  };
  const setPage = (n: number) => {
    const p = new URLSearchParams(params);
    if (n <= 0) p.delete("page");
    else p.set("page", String(n));
    setParams(p, { replace: true });
  };

  return (
    <SellerShell
      title="Invoices"
      subtitle="Everything you've submitted, newest first."
      action={
        <Button asChild size="sm" className="hidden sm:inline-flex">
          <Link to="/upload">
            <Upload className="h-4 w-4" /> Finance an invoice
          </Link>
        </Button>
      }
    >
      <div className="space-y-4 animate-fade-in">
        {/* Filter chips */}
        <div className="-mx-4 px-4 md:mx-0 md:px-0 overflow-x-auto">
          <div className="flex gap-2 w-max pb-1">
            {SELLER_FILTERS.map((f) => (
              <button
                key={f.key}
                type="button"
                onClick={() => setFilter(f.key)}
                className={cn(
                  "h-9 px-3.5 rounded-full border text-sm font-medium whitespace-nowrap transition-colors",
                  f.key === filter.key ? "bg-primary/15 border-primary text-primary" : "border-border text-muted-foreground hover:text-foreground hover:border-accent"
                )}
              >
                {f.label}
              </button>
            ))}
          </div>
        </div>

        <section data-nemo-card="" className="rounded-xl border bg-card">
          {q.isLoading ? (
            <LoadingState label="Loading invoices…" />
          ) : q.isError ? (
            <ErrorState error={q.error} onRetry={() => q.refetch()} />
          ) : rows.length === 0 ? (
            <EmptyState
              icon={Receipt}
              title={filter.key === "all" ? "No invoices yet" : `Nothing ${filter.label.toLowerCase()}`}
              description={filter.key === "all" ? "Upload an invoice to get an advance on it." : "Try another filter."}
              action={
                filter.key === "all" ? (
                  <Button asChild size="sm">
                    <Link to="/upload">Finance an invoice</Link>
                  </Button>
                ) : undefined
              }
            />
          ) : (
            <ul className="divide-y">
              {rows.map((inv) => {
                const days = daysUntil(inv.dueDate);
                return (
                  <li key={inv.id}>
                    <Link to={`/invoices/${inv.id}`} className="flex items-center gap-3 px-4 md:px-5 py-3.5 hover:bg-muted/50 transition-colors">
                      <div className="min-w-0 flex-1">
                        <p className="text-sm font-medium truncate">
                          {inv.buyerSnapshot?.name ?? "Buyer to confirm"}
                          {inv.invoiceNumber && <span className="text-muted-foreground font-normal"> · #{inv.invoiceNumber}</span>}
                        </p>
                        <p className="text-xs text-muted-foreground mt-0.5 truncate">
                          {statusHint(inv.status) || inv.status}
                          {inv.dueDate && (
                            <>
                              {" "}
                              · due {formatDate(inv.dueDate)}
                              {days != null && ["FUNDED", "PARTIALLY_PAID", "OVERDUE"].includes(inv.status) && (
                                <span className={cn(days < 0 ? "text-destructive" : days <= 7 ? "text-warning" : "")}> ({days < 0 ? `${-days}d late` : `${days}d`})</span>
                              )}
                            </>
                          )}
                        </p>
                      </div>
                      <div className="text-right shrink-0">
                        <p className="font-mono tabular-nums text-sm font-semibold">{formatMoney(inv.total, inv.currency)}</p>
                        <SellerStatusPill status={inv.status} className="mt-1" />
                      </div>
                      <ChevronRight className="h-4 w-4 text-muted-foreground shrink-0 hidden sm:block" />
                    </Link>
                  </li>
                );
              })}
            </ul>
          )}
        </section>

        {totalPages > 1 && (
          <div className="flex items-center justify-between text-sm text-muted-foreground">
            <Button variant="outline" size="sm" disabled={page <= 0} onClick={() => setPage(page - 1)}>
              <ChevronLeft className="h-4 w-4" /> Newer
            </Button>
            <span>
              Page {page + 1} of {totalPages}
            </span>
            <Button variant="outline" size="sm" disabled={page + 1 >= totalPages} onClick={() => setPage(page + 1)}>
              Older <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        )}
      </div>
    </SellerShell>
  );
};

export default SellerInvoicesPage;
