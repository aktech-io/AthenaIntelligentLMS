import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Banknote, Clock, AlertTriangle, CalendarRange, Plus, ArrowRight } from "lucide-react";
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell, CartesianGrid } from "recharts";
import { DashboardLayout } from "@/components/DashboardLayout";
import { StatCard } from "@/components/StatCard";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { invoiceService, INVOICE_STATUSES, type Stats } from "@/services/invoiceService";
import { formatMoney } from "@/lib/format";
import { LoadingState, ErrorState, EmptyState } from "./components/States";
import { InvoiceStatusPill } from "./components/InvoiceStatusPill";

const FUNNEL: string[] = ["UPLOADED", "EXTRACTED", "AWAITING_BUYER", "BUYER_CONFIRMED", "SCORED", "DECIDED", "OFFERED", "ACCEPTED", "FUNDED", "PAID"];
const TERMINAL_BAD: string[] = ["BUYER_DISPUTED", "OVERDUE", "DECLINED", "CANCELLED"];
const TIER_ORDER = ["A", "B", "C", "D", "UNRATED"];

const tierColor: Record<string, string> = {
  A: "hsl(var(--chart-2))",
  B: "hsl(var(--chart-2) / 0.7)",
  C: "hsl(var(--chart-1))",
  D: "hsl(var(--chart-5))",
  UNRATED: "hsl(var(--chart-4))",
};

const InvoiceFinanceOverviewPage = () => {
  const statsQ = useQuery<Stats>({ queryKey: ["if-stats"], queryFn: () => invoiceService.stats(), refetchInterval: 30_000 });
  const stats = statsQ.data;
  const ccy = stats?.currency ?? "KES";

  const funnelMax = Math.max(1, ...FUNNEL.map((s) => stats?.byStatus?.[s] ?? 0));
  const totalInvoices = Object.values(stats?.byStatus ?? {}).reduce((a, b) => a + (b ?? 0), 0);
  const tiers = TIER_ORDER.map((t) => stats?.byBuyerTier?.find((x) => x.tier === t)).filter(Boolean) as Stats["byBuyerTier"];
  const extraTiers = (stats?.byBuyerTier ?? []).filter((t) => !TIER_ORDER.includes(String(t.tier)));
  const tierData = [...tiers, ...extraTiers].map((t) => ({ ...t, name: t.tier === "UNRATED" ? "Unrated" : `Tier ${t.tier}` }));

  return (
    <DashboardLayout
      title="Invoice Finance"
      subtitle="Confirmed-invoice advances — funnel, exposure and buyer mix"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Invoice Finance" }]}
    >
      <div className="space-y-6 animate-fade-in">
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Button asChild variant="outline" size="sm">
            <Link to="/invoice-finance/queue">Review queue</Link>
          </Button>
          <Button asChild size="sm">
            <Link to="/invoice-finance/invoices?new=1">
              <Plus className="h-4 w-4 mr-1" /> New invoice
            </Link>
          </Button>
        </div>

        {statsQ.isLoading ? (
          <LoadingState label="Loading portfolio stats…" />
        ) : statsQ.isError ? (
          <ErrorState error={statsQ.error} onRetry={() => statsQ.refetch()} />
        ) : stats ? (
          <>
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 tabular-nums">
              <StatCard
                title="Funded"
                value={formatMoney(stats.fundedAmount, ccy, { compact: true })}
                subtitle={stats.fundedCount ? `${stats.fundedCount} advances` : undefined}
                icon={Banknote}
              />
              <StatCard title="Outstanding" value={formatMoney(stats.outstandingAmount, ccy, { compact: true })} icon={Clock} />
              <StatCard
                title="Overdue"
                value={formatMoney(stats.overdueAmount, ccy, { compact: true })}
                subtitle={stats.overdueCount ? `${stats.overdueCount} positions` : undefined}
                changeType={stats.overdueAmount > 0 ? "negative" : "neutral"}
                icon={AlertTriangle}
              />
              <StatCard
                title="Avg tenor"
                value={stats.avgTenorDays ? `${Math.round(stats.avgTenorDays)} days` : "—"}
                icon={CalendarRange}
              />
            </div>

            <div className="grid grid-cols-1 xl:grid-cols-5 gap-4">
              {/* Funnel */}
              <Card className="xl:col-span-3">
                <CardHeader className="pb-2 flex flex-row items-center justify-between">
                  <CardTitle className="text-sm font-heading">Pipeline funnel</CardTitle>
                  <span className="text-xs text-muted-foreground tabular-nums">{totalInvoices} invoices</span>
                </CardHeader>
                <CardContent>
                  {totalInvoices === 0 ? (
                    <EmptyState
                      title="No invoices yet"
                      description="Upload the first invoice on behalf of a seller to start the pipeline."
                      action={
                        <Button asChild size="sm">
                          <Link to="/invoice-finance/invoices?new=1">New invoice</Link>
                        </Button>
                      }
                    />
                  ) : (
                    <div className="space-y-2">
                      {FUNNEL.map((s) => {
                        const n = stats.byStatus?.[s] ?? 0;
                        const pct = Math.max(2, Math.round((n / funnelMax) * 100));
                        return (
                          <Link
                            key={s}
                            to={`/invoice-finance/invoices?status=${s}`}
                            className="grid grid-cols-[130px_1fr_40px] sm:grid-cols-[160px_1fr_48px] items-center gap-3 group"
                          >
                            <InvoiceStatusPill status={s} className="justify-self-start" />
                            <div className="h-5 rounded-md bg-muted overflow-hidden">
                              <div
                                className="h-full rounded-md bg-accent/70 group-hover:bg-accent transition-colors"
                                style={{ width: `${pct}%`, opacity: n === 0 ? 0.25 : 1 }}
                              />
                            </div>
                            <span className="text-sm tabular-nums text-right font-mono">{n}</span>
                          </Link>
                        );
                      })}
                      <div className="pt-3 mt-3 border-t flex flex-wrap gap-x-5 gap-y-1">
                        {TERMINAL_BAD.concat(INVOICE_STATUSES.filter((s) => !FUNNEL.includes(s) && !TERMINAL_BAD.includes(s))).map((s) => (
                          <Link key={s} to={`/invoice-finance/invoices?status=${s}`} className="flex items-center gap-2 text-xs">
                            <InvoiceStatusPill status={s} />
                            <span className="font-mono tabular-nums text-muted-foreground">{stats.byStatus?.[s] ?? 0}</span>
                          </Link>
                        ))}
                      </div>
                    </div>
                  )}
                </CardContent>
              </Card>

              {/* By buyer tier */}
              <Card className="xl:col-span-2">
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-heading">Funded by buyer tier</CardTitle>
                </CardHeader>
                <CardContent>
                  {tierData.length === 0 || tierData.every((t) => !t.amount && !t.count) ? (
                    <EmptyState title="No funded advances" description="Tier mix appears once advances are funded." />
                  ) : (
                    <div className="h-64">
                      <ResponsiveContainer width="100%" height="100%">
                        <BarChart data={tierData} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                          <CartesianGrid vertical={false} stroke="hsl(var(--border))" strokeDasharray="3 3" />
                          <XAxis dataKey="name" tick={{ fontSize: 11, fill: "hsl(var(--muted-foreground))" }} axisLine={false} tickLine={false} />
                          <YAxis
                            tick={{ fontSize: 11, fill: "hsl(var(--muted-foreground))" }}
                            axisLine={false}
                            tickLine={false}
                            width={52}
                            tickFormatter={(v) => formatMoney(v, ccy, { compact: true }).replace(ccy, "").trim()}
                          />
                          <Tooltip
                            cursor={{ fill: "hsl(var(--muted) / 0.6)" }}
                            contentStyle={{
                              background: "hsl(var(--popover))",
                              border: "1px solid hsl(var(--border))",
                              borderRadius: 8,
                              fontSize: 12,
                              color: "hsl(var(--popover-foreground))",
                            }}
                            formatter={(v: number, _n, p) => [`${formatMoney(v, ccy)} · ${p.payload.count} invoices`, "Funded"]}
                          />
                          <Bar dataKey="amount" radius={[6, 6, 0, 0]}>
                            {tierData.map((t) => (
                              <Cell key={String(t.tier)} fill={tierColor[String(t.tier)] ?? "hsl(var(--chart-3))"} />
                            ))}
                          </Bar>
                        </BarChart>
                      </ResponsiveContainer>
                    </div>
                  )}
                  <Link to="/invoice-finance/buyers" className="mt-3 inline-flex items-center gap-1 text-xs text-primary hover:underline">
                    Manage buyers <ArrowRight className="h-3 w-3" />
                  </Link>
                </CardContent>
              </Card>
            </div>
          </>
        ) : null}
      </div>
    </DashboardLayout>
  );
};

export default InvoiceFinanceOverviewPage;
