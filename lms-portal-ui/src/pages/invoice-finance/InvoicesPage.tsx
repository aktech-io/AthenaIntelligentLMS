import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Receipt, ChevronLeft, ChevronRight, Filter, X } from "lucide-react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { invoiceService, INVOICE_STATUSES, type Invoice, type InvoiceStatus } from "@/services/invoiceService";
import { formatMoney, formatDate } from "@/lib/format";
import { InvoiceStatusPill } from "./components/InvoiceStatusPill";
import { LoadingState, ErrorState, EmptyState } from "./components/States";
import { NewInvoiceDialog } from "./components/NewInvoiceDialog";
import { useInvoiceRole } from "./components/useInvoiceRole";

const PAGE_SIZE = 20;

const InvoicesPage = () => {
  const { isOfficer } = useInvoiceRole();
  const qc = useQueryClient();
  const [params, setParams] = useSearchParams();
  const status = (params.get("status") ?? "") as InvoiceStatus | "";
  const sellerId = params.get("sellerId") ?? "";
  const buyerId = params.get("buyerId") ?? "";
  const page = Number(params.get("page") ?? 0) || 0;
  const [sellerInput, setSellerInput] = useState(sellerId);
  const [newOpen, setNewOpen] = useState(params.get("new") === "1");

  useEffect(() => setSellerInput(sellerId), [sellerId]);

  const setParam = (k: string, v: string) => {
    const p = new URLSearchParams(params);
    if (v) p.set(k, v);
    else p.delete(k);
    if (k !== "page") p.delete("page");
    setParams(p, { replace: true });
  };

  const q = useQuery({
    queryKey: ["if-invoices", { status, sellerId, buyerId, page }],
    queryFn: () => invoiceService.list({ status, sellerId: sellerId || undefined, buyerId: buyerId || undefined, page, size: PAGE_SIZE }),
    refetchInterval: 15_000,
  });

  const rows: Invoice[] = q.data?.content ?? [];
  const hasFilters = !!(status || sellerId || buyerId);

  const daysToDue = (inv: Invoice) => {
    if (!inv.dueDate) return null;
    const d = Math.round((new Date(inv.dueDate).getTime() - Date.now()) / 86_400_000);
    return d;
  };

  return (
    <DashboardLayout
      title="Invoices"
      subtitle="Every invoice submitted for financing, across the pipeline"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Invoice Finance", href: "/invoice-finance" }, { label: "Invoices" }]}
    >
      <div className="space-y-4 animate-fade-in">
        <div className="flex flex-col sm:flex-row sm:items-center gap-2">
          <div className="flex flex-1 flex-wrap items-center gap-2">
            <Filter className="h-4 w-4 text-muted-foreground hidden sm:block" />
            <Select value={status || "ALL"} onValueChange={(v) => setParam("status", v === "ALL" ? "" : v)}>
              <SelectTrigger className="h-9 w-[180px] text-xs">
                <SelectValue placeholder="All statuses" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="ALL">All statuses</SelectItem>
                {INVOICE_STATUSES.map((s) => (
                  <SelectItem key={s} value={s}>
                    {s.replace(/_/g, " ")}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                setParam("sellerId", sellerInput.trim());
              }}
            >
              <Input value={sellerInput} onChange={(e) => setSellerInput(e.target.value)} placeholder="Seller customer ID" className="h-9 w-[180px] text-xs font-mono" />
            </form>
            {buyerId && (
              <Button variant="outline" size="sm" className="h-9 text-xs" onClick={() => setParam("buyerId", "")}>
                Buyer: {buyerId.slice(0, 8)}… <X className="h-3 w-3 ml-1" />
              </Button>
            )}
            {hasFilters && (
              <Button variant="ghost" size="sm" className="h-9 text-xs" onClick={() => setParams(new URLSearchParams(), { replace: true })}>
                Clear
              </Button>
            )}
          </div>
          <Button size="sm" onClick={() => setNewOpen(true)} disabled={!isOfficer}>
            <Plus className="h-4 w-4 mr-1" /> New invoice
          </Button>
        </div>

        <Card>
          <CardContent className="p-0">
            {q.isLoading ? (
              <LoadingState label="Loading invoices…" />
            ) : q.isError ? (
              <ErrorState error={q.error} onRetry={() => q.refetch()} />
            ) : rows.length === 0 ? (
              <EmptyState
                icon={Receipt}
                title={hasFilters ? "No invoices match these filters" : "No invoices yet"}
                description={hasFilters ? "Try clearing a filter." : "Upload the first invoice on behalf of a seller."}
                action={
                  !hasFilters && isOfficer ? (
                    <Button size="sm" onClick={() => setNewOpen(true)}>
                      <Plus className="h-4 w-4 mr-1" /> New invoice
                    </Button>
                  ) : undefined
                }
              />
            ) : (
              <>
                <div className="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Invoice</TableHead>
                        <TableHead>Seller</TableHead>
                        <TableHead>Buyer</TableHead>
                        <TableHead className="text-right">Total</TableHead>
                        <TableHead>Issued</TableHead>
                        <TableHead>Due</TableHead>
                        <TableHead>Status</TableHead>
                        <TableHead>Updated</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {rows.map((inv) => {
                        const dtd = daysToDue(inv);
                        return (
                          <TableRow key={inv.id} className="table-row-hover">
                            <TableCell>
                              <Link to={`/invoice-finance/invoices/${inv.id}`} className="font-medium hover:underline">
                                {inv.invoiceNumber ?? <span className="text-muted-foreground italic">pending extraction</span>}
                              </Link>
                              <div className="text-[11px] text-muted-foreground font-mono">{inv.id.slice(0, 8)}</div>
                            </TableCell>
                            <TableCell className="text-sm">
                              <div>{inv.seller?.name ?? "—"}</div>
                              <div className="text-xs text-muted-foreground font-mono">{inv.sellerCustomerId}</div>
                            </TableCell>
                            <TableCell className="text-sm">{inv.buyerSnapshot?.name ?? "—"}</TableCell>
                            <TableCell className="text-right tabular-nums font-mono text-sm">{formatMoney(inv.total, inv.currency)}</TableCell>
                            <TableCell className="text-xs whitespace-nowrap">{formatDate(inv.issueDate)}</TableCell>
                            <TableCell className="text-xs whitespace-nowrap">
                              {formatDate(inv.dueDate)}
                              {dtd !== null && !["PAID", "DECLINED", "CANCELLED"].includes(inv.status) && (
                                <span className={`ml-1 ${dtd < 0 ? "text-destructive" : dtd < 7 ? "text-warning" : "text-muted-foreground"}`}>
                                  ({dtd < 0 ? `${-dtd}d late` : `${dtd}d`})
                                </span>
                              )}
                            </TableCell>
                            <TableCell>
                              <InvoiceStatusPill status={inv.status} />
                              {inv.statusReason && <div className="text-[11px] text-muted-foreground mt-0.5 font-mono">{inv.statusReason}</div>}
                            </TableCell>
                            <TableCell className="text-xs text-muted-foreground whitespace-nowrap">{formatDate(inv.updatedAt)}</TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                </div>
                <div className="flex items-center justify-between px-4 py-3 border-t text-xs text-muted-foreground">
                  <span className="tabular-nums">
                    {q.data!.totalElements} invoice{q.data!.totalElements === 1 ? "" : "s"} · page {page + 1} of {Math.max(1, q.data!.totalPages)}
                  </span>
                  <div className="flex gap-1">
                    <Button variant="outline" size="sm" className="h-7" disabled={page === 0} onClick={() => setParam("page", String(page - 1))}>
                      <ChevronLeft className="h-3.5 w-3.5" />
                    </Button>
                    <Button variant="outline" size="sm" className="h-7" disabled={q.data!.last} onClick={() => setParam("page", String(page + 1))}>
                      <ChevronRight className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>
              </>
            )}
          </CardContent>
        </Card>
      </div>

      <NewInvoiceDialog
        open={newOpen}
        onOpenChange={(o) => {
          setNewOpen(o);
          if (!o && params.get("new")) setParam("new", "");
          if (!o) {
            qc.invalidateQueries({ queryKey: ["if-invoices"] });
            qc.invalidateQueries({ queryKey: ["if-stats"] });
          }
        }}
        onCreated={() => qc.invalidateQueries({ queryKey: ["if-invoices"] })}
      />
    </DashboardLayout>
  );
};

export default InvoicesPage;
