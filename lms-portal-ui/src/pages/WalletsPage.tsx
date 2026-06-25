import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { PiggyBank, Wallet, ArrowUpRight, Search, CreditCard, Loader2 } from "lucide-react";
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { overdraftService, type CustomerWallet } from "@/services/overdraftService";
import { useToast } from "@/hooks/use-toast";

const statusStyle: Record<string, string> = {
  ACTIVE: "bg-success/10 text-success border-success/20",
  SUSPENDED: "bg-destructive/10 text-destructive border-destructive/20",
  CLOSED: "bg-muted text-muted-foreground border-border",
};

const WalletsPage = () => {
  const { toast } = useToast();
  const [search, setSearch] = useState("");
  const queryClient = useQueryClient();

  const { data: wallets = [], isLoading, error } = useQuery<CustomerWallet[]>({
    queryKey: ["wallets"],
    queryFn: () => overdraftService.listWallets(),
  });

  const applyOverdraftMutation = useMutation({
    mutationFn: (walletId: string) => overdraftService.applyOverdraft(walletId),
    onSuccess: (facility) => {
      toast({ title: `Overdraft approved! Band ${facility.creditBand} — KES ${Number(facility.approvedLimit).toLocaleString()} limit` });
      queryClient.invalidateQueries({ queryKey: ["wallets"] });
    },
    onError: (err: Error) => toast({ title: `Failed: ${err.message}`, variant: "destructive" }),
  });

  const filtered = wallets.filter(
    (w) =>
      w.customerId.toLowerCase().includes(search.toLowerCase()) ||
      w.accountNumber.toLowerCase().includes(search.toLowerCase())
  );

  const totalBalance = wallets.reduce((s, w) => s + Number(w.currentBalance), 0);
  const totalAvailable = wallets.reduce((s, w) => s + Number(w.availableBalance), 0);
  const activeCount = wallets.filter((w) => w.status === "ACTIVE").length;
  const totalDrawn = wallets.reduce((s, w) => s + Math.max(0, Number(w.availableBalance) - Number(w.currentBalance)), 0);

  return (
    <DashboardLayout
      title="Wallet Accounts"
      subtitle="Customer wallet accounts with overdraft facility"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Float & Wallet" }, { label: "Wallet Accounts" }]}
    >
      <div className="space-y-6 animate-fade-in">
        {/* KPI Bar */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Total Wallets</span>
                <PiggyBank className="h-4 w-4 text-muted-foreground" />
              </div>
              <p className="text-2xl font-heading">{wallets.length}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Total Available</span>
                <Wallet className="h-4 w-4 text-muted-foreground" />
              </div>
              <p className="text-2xl font-heading">KES {totalAvailable.toLocaleString()}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Net Overdraft Drawn</span>
                <CreditCard className="h-4 w-4 text-warning" />
              </div>
              <p className="text-2xl font-heading">KES {totalDrawn.toLocaleString()}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Active Wallets</span>
                <ArrowUpRight className="h-4 w-4 text-success" />
              </div>
              <p className="text-2xl font-heading">{activeCount}</p>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <CardTitle className="text-sm font-medium">All Wallets</CardTitle>
              <div className="relative w-56">
                <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground" />
                <Input
                  placeholder="Search wallets…"
                  className="pl-8 text-xs h-8"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                />
              </div>
            </div>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <div className="flex items-center justify-center py-12 text-muted-foreground">
                <Loader2 className="h-6 w-6 animate-spin mr-2" />
                <span className="text-sm">Loading wallets…</span>
              </div>
            ) : error ? (
              <p className="text-center text-sm text-destructive py-8">Failed to load wallets</p>
            ) : filtered.length === 0 ? (
              <p className="text-center text-sm text-muted-foreground py-8">No wallets found</p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">Account Number</TableHead>
                    <TableHead className="text-xs">Customer ID</TableHead>
                    <TableHead className="text-xs">Currency</TableHead>
                    <TableHead className="text-xs text-right">Balance</TableHead>
                    <TableHead className="text-xs text-right">Available</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                    <TableHead className="text-xs">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filtered.map((w) => (
                    <TableRow key={w.id} className="table-row-hover">
                      <TableCell className="text-xs font-mono">{w.accountNumber}</TableCell>
                      <TableCell className="text-xs font-medium">{w.customerId}</TableCell>
                      <TableCell className="text-xs">{w.currency}</TableCell>
                      <TableCell className={`text-xs text-right font-mono ${Number(w.currentBalance) < 0 ? "text-destructive" : ""}`}>
                        {Number(w.currentBalance).toLocaleString()}
                      </TableCell>
                      <TableCell className="text-xs text-right font-mono">
                        {Number(w.availableBalance).toLocaleString()}
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className={`text-[10px] ${statusStyle[w.status] || ""}`}>
                          {w.status}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Button
                          size="sm"
                          variant="outline"
                          className="text-xs h-7 px-2"
                          disabled={applyOverdraftMutation.isPending}
                          onClick={() => applyOverdraftMutation.mutate(w.id)}
                        >
                          {applyOverdraftMutation.isPending ? (
                            <Loader2 className="h-3 w-3 animate-spin" />
                          ) : (
                            "Apply Overdraft"
                          )}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
};

export default WalletsPage;
