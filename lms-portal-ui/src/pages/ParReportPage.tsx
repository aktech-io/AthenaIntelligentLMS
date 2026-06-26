import { useQuery } from "@tanstack/react-query";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { formatKES } from "@/lib/format";
import { loanManagementService, type ParBucket } from "@/services/loanManagementService";

const fmtPct = (n: number) =>
  `${(n ?? 0).toLocaleString("en-KE", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}%`;

interface ParStatProps {
  label: string;
  value: number;
  hint: string;
}

const ParStat = ({ label, value, hint }: ParStatProps) => {
  // PAR is a risk ratio — colour it by severity for at-a-glance reading.
  const tone =
    value >= 10
      ? "border-destructive/30 bg-destructive/5"
      : value >= 5
        ? "border-amber-400/30 bg-amber-50"
        : "border-success/30 bg-success/5";
  const valueTone =
    value >= 10 ? "text-destructive" : value >= 5 ? "text-amber-600" : "text-success";
  return (
    <Card className={`border-2 ${tone}`}>
      <CardContent className="p-4">
        <p className="text-[10px] uppercase tracking-widest text-muted-foreground font-sans">{label}</p>
        <p className={`text-2xl font-bold font-mono mt-1 ${valueTone}`}>{fmtPct(value)}</p>
        <p className="text-[11px] text-muted-foreground mt-1 font-sans">{hint}</p>
      </CardContent>
    </Card>
  );
};

const ParReportPage = () => {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["loans", "par-report"],
    queryFn: () => loanManagementService.getParReport(),
    staleTime: 300_000,
    retry: false,
  });

  const buckets: ParBucket[] = data?.buckets ?? [];
  const totalOutstanding = data?.totalOutstanding ?? 0;

  return (
    <DashboardLayout
      title="Portfolio at Risk"
      subtitle="Loan ageing buckets and PAR ratios"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Reports" }, { label: "Portfolio at Risk" }]}
    >
      <div className="space-y-4 max-w-5xl">
        {data?.asOf && (
          <div className="flex items-center justify-between text-sm text-muted-foreground">
            <span>As of {new Date(data.asOf).toLocaleDateString("en-KE")}</span>
            <span>
              {data.activeLoans.toLocaleString("en-KE")} active loans · {formatKES(totalOutstanding)} outstanding
            </span>
          </div>
        )}

        {/* PAR ratios */}
        {isLoading ? (
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-28 w-full" />
            ))}
          </div>
        ) : isError ? null : (
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
            <ParStat label="PAR 1" value={data?.par1 ?? 0} hint="Overdue 1+ days" />
            <ParStat label="PAR 30" value={data?.par30 ?? 0} hint="Overdue 30+ days" />
            <ParStat label="PAR 60" value={data?.par60 ?? 0} hint="Overdue 60+ days" />
            <ParStat label="PAR 90" value={data?.par90 ?? 0} hint="Overdue 90+ days" />
          </div>
        )}

        {/* Ageing buckets */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Ageing Buckets</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 5 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : isError ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">Unable to load PAR report</p>
                <p className="text-xs mt-1">Loan service returned an error.</p>
              </div>
            ) : buckets.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">No active loans</p>
                <p className="text-xs mt-1">No portfolio data available.</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-[10px] font-sans">Bucket</TableHead>
                    <TableHead className="text-[10px] font-sans text-right">Loans</TableHead>
                    <TableHead className="text-[10px] font-sans text-right">Outstanding (KES)</TableHead>
                    <TableHead className="text-[10px] font-sans text-right">% of Portfolio</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {buckets.map((row) => {
                    const share = totalOutstanding > 0 ? (row.outstanding / totalOutstanding) * 100 : 0;
                    return (
                      <TableRow key={row.bucket} className="table-row-hover">
                        <TableCell className="text-xs font-sans font-medium">{row.bucket}</TableCell>
                        <TableCell className="text-xs font-mono text-right">
                          {row.loans.toLocaleString("en-KE")}
                        </TableCell>
                        <TableCell className="text-xs font-mono text-right">
                          {formatKES(row.outstanding)}
                        </TableCell>
                        <TableCell className="text-xs font-mono text-right text-muted-foreground">
                          {fmtPct(share)}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                  <TableRow className="border-t-2 bg-muted/30">
                    <TableCell className="text-xs font-sans font-bold">Total</TableCell>
                    <TableCell className="text-sm font-mono text-right font-bold">
                      {(data?.activeLoans ?? 0).toLocaleString("en-KE")}
                    </TableCell>
                    <TableCell className="text-sm font-mono text-right font-bold">
                      {formatKES(totalOutstanding)}
                    </TableCell>
                    <TableCell className="text-xs font-mono text-right font-bold">100.00%</TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
};

export default ParReportPage;
