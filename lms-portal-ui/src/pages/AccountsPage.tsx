import { useState, useMemo } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { accountService, type Account } from "@/services/accountService";
import { Search, Plus, ChevronLeft, ChevronRight } from "lucide-react";

// ─── Helpers ──────────────────────────────────────────

const ACCOUNT_TYPE_TABS = ["All", "CURRENT", "SAVINGS", "FIXED_DEPOSIT", "WALLET"] as const;
const TAB_LABELS: Record<string, string> = {
  All: "All",
  CURRENT: "Current",
  SAVINGS: "Savings",
  FIXED_DEPOSIT: "Fixed Deposit",
  WALLET: "Wallet",
};

const STATUS_OPTIONS = ["All", "ACTIVE", "DORMANT", "FROZEN", "CLOSED", "PENDING_APPROVAL"] as const;

const statusColors: Record<string, string> = {
  ACTIVE: "bg-green-100 text-green-700 border-green-300",
  DORMANT: "bg-amber-100 text-amber-700 border-amber-300",
  FROZEN: "bg-blue-100 text-blue-700 border-blue-300",
  CLOSED: "bg-red-100 text-red-700 border-red-300",
  PENDING_APPROVAL: "bg-purple-100 text-purple-700 border-purple-300",
};

function resolveBalance(
  balance: Account["balance"],
  currency: string
): string {
  if (balance == null) return "--";
  const num =
    typeof balance === "number"
      ? balance
      : balance.availableBalance ?? balance.currentBalance ?? 0;
  return `${currency} ${num.toLocaleString("en", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

function resolveRate(acc: Account): string {
  if (acc.interestRateOverride != null && acc.interestRateOverride > 0)
    return `${acc.interestRateOverride}%`;
  return "--";
}

const PAGE_SIZES = [10, 20, 50, 100];

// ─── Component ────────────────────────────────────────

const AccountsPage = () => {
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState<string>("All");
  const [searchQuery, setSearchQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<string>("All");
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(20);

  const { data: apiPage, isLoading } = useQuery({
    queryKey: ["accounts", "list", page, pageSize],
    queryFn: () => accountService.listAccounts(page, pageSize),
    staleTime: 60_000,
    retry: false,
  });

  const accounts: Account[] = apiPage?.content ?? [];

  // Client-side filtering for type, status, and search
  const filtered = useMemo(() => {
    let result = accounts;

    if (activeTab !== "All") {
      result = result.filter(
        (a) => a.accountType?.toUpperCase() === activeTab
      );
    }

    if (statusFilter !== "All") {
      result = result.filter(
        (a) => a.status?.toUpperCase() === statusFilter
      );
    }

    if (searchQuery.trim()) {
      const q = searchQuery.trim().toLowerCase();
      result = result.filter(
        (a) =>
          a.accountNumber?.toLowerCase().includes(q) ||
          a.accountName?.toLowerCase().includes(q) ||
          a.customerId?.toLowerCase().includes(q)
      );
    }

    return result;
  }, [accounts, activeTab, statusFilter, searchQuery]);

  const totalPages = apiPage?.totalPages ?? 1;
  const totalElements = apiPage?.totalElements ?? accounts.length;

  return (
    <DashboardLayout
      title="Account Services"
      subtitle="Deposits, wallets & account management"
    >
      <div className="space-y-4 animate-fade-in">
        {/* Tab bar */}
        <div className="flex flex-wrap items-center gap-1 border-b pb-2">
          {ACCOUNT_TYPE_TABS.map((tab) => (
            <Button
              key={tab}
              variant={activeTab === tab ? "default" : "ghost"}
              size="sm"
              className="text-xs font-sans"
              onClick={() => {
                setActiveTab(tab);
                setPage(0);
              }}
            >
              {TAB_LABELS[tab]}
            </Button>
          ))}
        </div>

        {/* Toolbar: Search, Status Filter, Open New */}
        <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3">
          <div className="relative flex-1 max-w-sm">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search by account # or customer..."
              className="pl-9 text-xs font-sans"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
          </div>

          <Select
            value={statusFilter}
            onValueChange={(val) => {
              setStatusFilter(val);
              setPage(0);
            }}
          >
            <SelectTrigger className="w-[160px] text-xs font-sans">
              <SelectValue placeholder="Status" />
            </SelectTrigger>
            <SelectContent>
              {STATUS_OPTIONS.map((s) => (
                <SelectItem key={s} value={s} className="text-xs">
                  {s === "All" ? "All Statuses" : s.replace("_", " ")}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Button
            size="sm"
            className="text-xs font-sans ml-auto"
            onClick={() => navigate("/account-opening")}
          >
            <Plus className="h-3.5 w-3.5 mr-1" />
            Open New Account
          </Button>
        </div>

        {/* Count */}
        <div className="text-sm text-muted-foreground font-sans">
          {isLoading
            ? "Loading accounts..."
            : `Showing ${filtered.length} of ${totalElements.toLocaleString()} accounts`}
        </div>

        {/* Table */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">
              Account Directory
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 8 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : filtered.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">No accounts found</p>
                <p className="text-xs mt-1">
                  {searchQuery || statusFilter !== "All" || activeTab !== "All"
                    ? "Try adjusting your filters."
                    : "No account records returned from the backend."}
                </p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[11px] uppercase tracking-wider">
                      Account #
                    </TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider">
                      Customer
                    </TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider">
                      Type
                    </TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider">
                      Product
                    </TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider text-right">
                      Balance
                    </TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider text-right">
                      Rate
                    </TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider">
                      Status
                    </TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider">
                      Last Activity
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filtered.map((acc) => (
                    <TableRow
                      key={acc.id}
                      className="table-row-hover cursor-pointer"
                      onClick={() => navigate(`/account/${acc.id}`)}
                    >
                      <TableCell className="text-xs font-mono">
                        {acc.accountNumber}
                      </TableCell>
                      <TableCell className="text-xs font-medium">
                        {acc.accountName ?? acc.customerId}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {acc.accountType?.replace("_", " ") ?? "--"}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {acc.depositProductId ?? "--"}
                      </TableCell>
                      <TableCell className="text-xs font-medium text-right font-mono">
                        {resolveBalance(acc.balance, acc.currency)}
                      </TableCell>
                      <TableCell className="text-xs font-mono text-right">
                        {resolveRate(acc)}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant="outline"
                          className={`text-[11px] font-semibold ${
                            statusColors[acc.status?.toUpperCase()] ??
                            "bg-muted text-muted-foreground"
                          }`}
                        >
                          {acc.status}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground font-sans">
                        {acc.lastTransactionDate
                          ? acc.lastTransactionDate.split("T")[0]
                          : "--"}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Pagination */}
        {!isLoading && totalPages > 0 && (
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground font-sans">
                Rows per page:
              </span>
              <Select
                value={String(pageSize)}
                onValueChange={(val) => {
                  setPageSize(Number(val));
                  setPage(0);
                }}
              >
                <SelectTrigger className="w-[70px] h-8 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PAGE_SIZES.map((s) => (
                    <SelectItem key={s} value={String(s)} className="text-xs">
                      {s}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground font-sans">
                Page {page + 1} of {totalPages}
              </span>
              <Button
                variant="outline"
                size="icon"
                className="h-8 w-8"
                disabled={page === 0}
                onClick={() => setPage((p) => Math.max(0, p - 1))}
              >
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <Button
                variant="outline"
                size="icon"
                className="h-8 w-8"
                disabled={page >= totalPages - 1}
                onClick={() => setPage((p) => p + 1)}
              >
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          </div>
        )}
      </div>
    </DashboardLayout>
  );
};

export default AccountsPage;
