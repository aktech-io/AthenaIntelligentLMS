import { useState, useMemo } from "react";
import { Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Building2, Pencil, Loader2, Search } from "lucide-react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useToast } from "@/hooks/use-toast";
import { invoiceService, type Buyer, type BuyerTier } from "@/services/invoiceService";
import { formatMoney, formatDate } from "@/lib/format";
import { TierPill } from "./components/InvoiceStatusPill";
import { LoadingState, ErrorState, EmptyState } from "./components/States";
import { useInvoiceRole } from "./components/useInvoiceRole";

const TIERS: BuyerTier[] = ["A", "B", "C", "D", "UNRATED"];

const BuyersPage = () => {
  const { isManager } = useInvoiceRole();
  const { toast } = useToast();
  const qc = useQueryClient();
  const q = useQuery<Buyer[]>({ queryKey: ["if-buyers"], queryFn: () => invoiceService.listBuyers() });
  const [search, setSearch] = useState("");
  const [editing, setEditing] = useState<Buyer | null>(null);
  const [tier, setTier] = useState<BuyerTier>("UNRATED");
  const [limit, setLimit] = useState("");

  const openEdit = (b: Buyer) => {
    setEditing(b);
    setTier(b.tier ?? "UNRATED");
    setLimit(b.limitAmount != null ? String(b.limitAmount) : "");
  };

  const save = useMutation({
    mutationFn: () => invoiceService.updateBuyer(editing!.id, { tier, limitAmount: limit === "" ? undefined : Number(limit) }),
    onSuccess: () => {
      toast({ title: "Buyer updated" });
      setEditing(null);
      qc.invalidateQueries({ queryKey: ["if-buyers"] });
    },
    onError: (e: Error) => toast({ title: "Update failed", description: e.message, variant: "destructive" }),
  });

  const rows = useMemo(() => {
    const s = search.trim().toLowerCase();
    const all = q.data ?? [];
    if (!s) return all;
    return all.filter((b) => [b.name, b.taxId, b.emailDomain].some((v) => v?.toLowerCase().includes(s)));
  }, [q.data, search]);

  return (
    <DashboardLayout
      title="Buyers"
      subtitle="Debtors whose confirmations back the advances — tier and exposure limits"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Invoice Finance", href: "/invoice-finance" }, { label: "Buyers" }]}
    >
      <div className="space-y-4 animate-fade-in">
        <div className="relative max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
          <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search name, tax ID, domain…" className="pl-9 h-9" />
        </div>
        <Card>
          <CardContent className="p-0">
            {q.isLoading ? (
              <LoadingState label="Loading buyers…" />
            ) : q.isError ? (
              <ErrorState error={q.error} onRetry={() => q.refetch()} />
            ) : rows.length === 0 ? (
              <EmptyState icon={Building2} title={search ? "No buyers match" : "No buyers yet"} description="Buyers are created automatically when an invoice is extracted." />
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Buyer</TableHead>
                      <TableHead>Tax ID</TableHead>
                      <TableHead>Tier</TableHead>
                      <TableHead className="text-right">Limit</TableHead>
                      <TableHead className="text-right">Exposure</TableHead>
                      <TableHead className="text-right">Utilisation</TableHead>
                      <TableHead className="text-right">On-time</TableHead>
                      <TableHead className="text-right">Confirmations</TableHead>
                      <TableHead>Since</TableHead>
                      <TableHead className="text-right"></TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {rows.map((b) => {
                      const util = b.limitAmount ? Math.min(100, Math.round(((b.exposureAmount ?? 0) / b.limitAmount) * 100)) : null;
                      return (
                        <TableRow key={b.id} className="table-row-hover">
                          <TableCell>
                            <Link to={`/invoice-finance/invoices?buyerId=${b.id}`} className="font-medium hover:underline">
                              {b.name}
                            </Link>
                            {b.emailDomain && <div className="text-xs text-muted-foreground">@{b.emailDomain}</div>}
                          </TableCell>
                          <TableCell className="font-mono text-xs">{b.taxId ?? "—"}</TableCell>
                          <TableCell>
                            <TierPill tier={b.tier} />
                          </TableCell>
                          <TableCell className="text-right tabular-nums font-mono text-sm">{b.limitAmount != null ? formatMoney(b.limitAmount) : "—"}</TableCell>
                          <TableCell className="text-right tabular-nums font-mono text-sm">{formatMoney(b.exposureAmount ?? 0)}</TableCell>
                          <TableCell className="text-right">
                            {util === null ? (
                              <span className="text-xs text-muted-foreground">—</span>
                            ) : (
                              <div className="flex items-center justify-end gap-2">
                                <div className="h-1.5 w-16 rounded-full bg-muted overflow-hidden">
                                  <div className={`h-full ${util > 90 ? "bg-destructive" : util > 70 ? "bg-warning" : "bg-accent"}`} style={{ width: `${util}%` }} />
                                </div>
                                <span className="text-xs tabular-nums w-8 text-right">{util}%</span>
                              </div>
                            )}
                          </TableCell>
                          <TableCell className="text-right tabular-nums text-sm">{b.onTimeRate != null ? `${Math.round(b.onTimeRate * (b.onTimeRate <= 1 ? 100 : 1))}%` : "—"}</TableCell>
                          <TableCell className="text-right tabular-nums text-sm">{b.confirmationsCount ?? 0}</TableCell>
                          <TableCell className="text-xs text-muted-foreground whitespace-nowrap">{formatDate(b.createdAt)}</TableCell>
                          <TableCell className="text-right">
                            <Button size="sm" variant="ghost" className="h-8" disabled={!isManager} onClick={() => openEdit(b)} title={isManager ? "Edit tier / limit" : "MANAGER role required"}>
                              <Pencil className="h-3.5 w-3.5" />
                            </Button>
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <Dialog open={!!editing} onOpenChange={(o) => !o && setEditing(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="font-heading">Edit buyer</DialogTitle>
            <DialogDescription>{editing?.name} — tier drives the advance rate table; the limit caps total exposure.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="space-y-2">
              <Label>Tier</Label>
              <Select value={tier} onValueChange={(v) => setTier(v as BuyerTier)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TIERS.map((t) => (
                    <SelectItem key={t} value={t}>
                      {t === "UNRATED" ? "Unrated (treated as C)" : `Tier ${t}`}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="buyer-limit">Exposure limit</Label>
              <Input id="buyer-limit" type="number" min={0} step={1000} value={limit} onChange={(e) => setLimit(e.target.value)} placeholder="Leave blank for no limit" className="font-mono" />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditing(null)} disabled={save.isPending}>
              Cancel
            </Button>
            <Button onClick={() => save.mutate()} disabled={save.isPending}>
              {save.isPending && <Loader2 className="h-4 w-4 mr-1 animate-spin" />} Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
};

export default BuyersPage;
