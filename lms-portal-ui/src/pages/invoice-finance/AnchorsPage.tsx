import { useQuery } from "@tanstack/react-query";
import { Anchor as AnchorIcon } from "lucide-react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { invoiceService, shortHash, type Anchor } from "@/services/invoiceService";
import { formatDateTime } from "@/lib/format";
import { LoadingState, ErrorState, EmptyState } from "./components/States";
import { CopyButton } from "./components/CopyButton";

const AnchorsPage = () => {
  const q = useQuery<Anchor[]>({ queryKey: ["if-anchors"], queryFn: () => invoiceService.anchors() });
  const rows = [...(q.data ?? [])].sort((a, b) => b.toSeq - a.toSeq);

  return (
    <DashboardLayout
      title="Anchors"
      subtitle="Merkle roots of the invoice ledger, periodically anchored for independent verification"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Invoice Finance", href: "/invoice-finance" }, { label: "Anchors" }]}
    >
      <div className="space-y-4 animate-fade-in">
        <Card>
          <CardContent className="p-0">
            {q.isLoading ? (
              <LoadingState label="Loading anchors…" />
            ) : q.isError ? (
              <ErrorState error={q.error} onRetry={() => q.refetch()} />
            ) : rows.length === 0 ? (
              <EmptyState icon={AnchorIcon} title="No anchors yet" description="The anchoring job publishes a Merkle root over new ledger events on its schedule." />
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Anchored</TableHead>
                      <TableHead>Merkle root</TableHead>
                      <TableHead className="text-right">Events (seq)</TableHead>
                      <TableHead>Method</TableHead>
                      <TableHead>Receipt</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {rows.map((a) => (
                      <TableRow key={a.id} className="table-row-hover">
                        <TableCell className="whitespace-nowrap text-sm">{formatDateTime(a.anchoredAt)}</TableCell>
                        <TableCell>
                          <span className="inline-flex items-center font-mono text-xs">
                            {shortHash(a.merkleRoot, 16)}
                            <CopyButton value={a.merkleRoot} />
                          </span>
                        </TableCell>
                        <TableCell className="text-right font-mono text-xs tabular-nums">
                          {a.fromSeq}–{a.toSeq} <span className="text-muted-foreground">({a.toSeq - a.fromSeq + 1})</span>
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline" className={a.method === "OTS" ? "bg-accent/15 text-accent border-accent/40 font-mono text-[11px]" : "font-mono text-[11px] text-muted-foreground"}>
                            {a.method === "OTS" ? "OpenTimestamps" : a.method}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground">{a.receipt ? `${Math.round((a.receipt.length * 3) / 4)} bytes` : "—"}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
};

export default AnchorsPage;
