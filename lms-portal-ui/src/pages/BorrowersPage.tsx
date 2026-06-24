import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Plus, Search, Download, User, ChevronLeft, ChevronRight, ShieldCheck } from "lucide-react";
import { customerService, type CreateCustomerRequest } from "@/services/customerService";
import { useToast } from "@/hooks/use-toast";

const PAGE_SIZE = 20;

const statusColors: Record<string, string> = {
  ACTIVE: "bg-success/15 text-success border-success/30",
  INACTIVE: "bg-muted text-muted-foreground border-border",
  SUSPENDED: "bg-destructive/15 text-destructive border-destructive/30",
  BLOCKED: "bg-destructive/15 text-destructive border-destructive/30",
};

const kycColors: Record<string, string> = {
  VERIFIED: "bg-success/15 text-success border-success/30",
  PENDING: "bg-warning/15 text-warning border-warning/30",
  REJECTED: "bg-destructive/15 text-destructive border-destructive/30",
};

const BorrowersPage = () => {
  const navigate = useNavigate();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [statusFilter, setStatusFilter] = useState<string>("ALL");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [form, setForm] = useState<CreateCustomerRequest>({
    customerId: "",
    firstName: "",
    lastName: "",
    email: "",
    phone: "",
    customerType: "INDIVIDUAL",
  });

  const { data, isLoading, isError } = useQuery({
    queryKey: ["customers", page, PAGE_SIZE],
    queryFn: () => customerService.listCustomers(page, PAGE_SIZE),
  });

  const searchQuery = useQuery({
    queryKey: ["customers-search", search],
    queryFn: () => customerService.searchCustomers(search),
    enabled: search.length >= 2,
  });

  const createMutation = useMutation({
    mutationFn: (req: CreateCustomerRequest) => customerService.createCustomer(req),
    onSuccess: () => {
      toast({ title: "Customer created successfully" });
      queryClient.invalidateQueries({ queryKey: ["customers"] });
      setDialogOpen(false);
      setForm({ customerId: "", firstName: "", lastName: "", email: "", phone: "", customerType: "INDIVIDUAL" });
    },
    onError: (err: Error) => {
      toast({ title: "Failed to create customer", description: err.message, variant: "destructive" });
    },
  });

  const verifyKycMutation = useMutation({
    mutationFn: (id: string) => customerService.updateKyc(id, "VERIFIED"),
    onSuccess: () => {
      toast({ title: "KYC Verified" });
      queryClient.invalidateQueries({ queryKey: ["customers"] });
      queryClient.invalidateQueries({ queryKey: ["customers-search"] });
    },
    onError: (err: Error) => {
      toast({ title: "Failed to verify KYC", description: err.message, variant: "destructive" });
    },
  });

  const isSearching = search.length >= 2;
  const rawCustomers = isSearching ? (searchQuery.data ?? []) : (data?.content ?? []);
  const customers = statusFilter === "ALL"
    ? rawCustomers
    : rawCustomers.filter((c) => c.status === statusFilter);
  const loading = isSearching ? searchQuery.isLoading : isLoading;
  const error = isSearching ? searchQuery.isError : isError;
  const totalPages = data?.totalPages ?? 1;
  const totalElements = data?.totalElements ?? 0;

  const handleExport = () => {
    const rows = [
      ["Customer ID", "First Name", "Last Name", "Phone", "Email", "Type", "KYC Status", "Status", "Created"],
      ...customers.map((c) => [
        c.customerId, c.firstName, c.lastName, c.phone ?? "", c.email ?? "",
        c.customerType, c.kycStatus ?? "", c.status, c.createdAt ?? "",
      ]),
    ];
    const csv = rows.map((r) => r.map((v) => `"${v}"`).join(",")).join("\n");
    const blob = new Blob([csv], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `customers_export_${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <DashboardLayout title="Customers" subtitle="Client directory & KYC management">
      <div className="space-y-4 animate-fade-in">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <div className="relative w-64">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
              <Input
                placeholder="Search customers..."
                className="pl-8 h-9 text-xs"
                value={search}
                onChange={(e) => { setSearch(e.target.value); setPage(0); }}
              />
            </div>
            <select
              className="h-9 rounded-md border text-xs px-2"
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
            >
              <option value="ALL">All Statuses</option>
              <option value="ACTIVE">Active</option>
              <option value="INACTIVE">Inactive</option>
              <option value="SUSPENDED">Suspended</option>
              <option value="BLOCKED">Blocked</option>
            </select>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" className="text-xs" onClick={handleExport}>
              <Download className="mr-1.5 h-3.5 w-3.5" /> Export
            </Button>
            <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
              <DialogTrigger asChild>
                <Button size="sm" className="text-xs bg-primary hover:bg-primary/90">
                  <Plus className="mr-1.5 h-3.5 w-3.5" /> Add Customer
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Create Customer</DialogTitle>
                </DialogHeader>
                <div className="grid gap-3 py-2">
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <Label className="text-xs">Customer ID *</Label>
                      <Input className="text-xs mt-1" value={form.customerId}
                        onChange={(e) => setForm({ ...form, customerId: e.target.value })} />
                    </div>
                    <div>
                      <Label className="text-xs">Type</Label>
                      <select className="w-full h-9 rounded-md border text-xs px-2 mt-1"
                        value={form.customerType}
                        onChange={(e) => setForm({ ...form, customerType: e.target.value })}>
                        <option value="INDIVIDUAL">Individual</option>
                        <option value="BUSINESS">Business</option>
                      </select>
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <Label className="text-xs">First Name *</Label>
                      <Input className="text-xs mt-1" value={form.firstName}
                        onChange={(e) => setForm({ ...form, firstName: e.target.value })} />
                    </div>
                    <div>
                      <Label className="text-xs">Last Name *</Label>
                      <Input className="text-xs mt-1" value={form.lastName}
                        onChange={(e) => setForm({ ...form, lastName: e.target.value })} />
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <Label className="text-xs">Email</Label>
                      <Input className="text-xs mt-1" value={form.email}
                        onChange={(e) => setForm({ ...form, email: e.target.value })} />
                    </div>
                    <div>
                      <Label className="text-xs">Phone</Label>
                      <Input className="text-xs mt-1" value={form.phone}
                        onChange={(e) => setForm({ ...form, phone: e.target.value })} />
                    </div>
                  </div>
                  <Button size="sm" className="text-xs mt-2"
                    disabled={!form.customerId || !form.firstName || !form.lastName || createMutation.isPending}
                    onClick={() => createMutation.mutate(form)}>
                    {createMutation.isPending ? "Creating..." : "Create Customer"}
                  </Button>
                </div>
              </DialogContent>
            </Dialog>
          </div>
        </div>

        <Card>
          <CardContent className="p-0">
            {loading ? (
              <div className="flex items-center justify-center h-32 text-muted-foreground text-xs">
                Loading customers...
              </div>
            ) : error ? (
              <div className="flex items-center justify-center h-32 text-destructive text-xs">
                Failed to load customers.
              </div>
            ) : customers.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-32 text-muted-foreground">
                <p className="text-sm font-medium">No customers found</p>
                <p className="text-xs mt-1">Create a customer to get started.</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[10px] uppercase tracking-wider">Customer ID</TableHead>
                    <TableHead className="text-[10px] uppercase tracking-wider">Name</TableHead>
                    <TableHead className="text-[10px] uppercase tracking-wider">Phone</TableHead>
                    <TableHead className="text-[10px] uppercase tracking-wider">Email</TableHead>
                    <TableHead className="text-[10px] uppercase tracking-wider">Type</TableHead>
                    <TableHead className="text-[10px] uppercase tracking-wider">KYC</TableHead>
                    <TableHead className="text-[10px] uppercase tracking-wider">Status</TableHead>
                    <TableHead className="text-[10px] uppercase tracking-wider">Created</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {customers.map((cust) => (
                    <TableRow
                      key={cust.id}
                      className="table-row-hover cursor-pointer"
                      onClick={() => navigate(`/customer/${cust.id}`)}
                    >
                      <TableCell className="text-xs font-mono">{cust.customerId}</TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <div className="h-7 w-7 rounded-full bg-primary/10 flex items-center justify-center">
                            <User className="h-3.5 w-3.5 text-primary" />
                          </div>
                          <span className="text-xs font-medium">
                            {cust.firstName} {cust.lastName}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">{cust.phone ?? "—"}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">{cust.email ?? "—"}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">{cust.customerType}</TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <Badge variant="outline"
                            className={`text-[10px] font-semibold ${kycColors[cust.kycStatus ?? ""] ?? "bg-muted text-muted-foreground border-border"}`}>
                            {cust.kycStatus ?? "—"}
                          </Badge>
                          {cust.kycStatus !== "VERIFIED" && (
                            <Button
                              variant="outline"
                              size="sm"
                              className="h-6 px-2 text-[10px]"
                              disabled={verifyKycMutation.isPending && verifyKycMutation.variables === cust.id}
                              onClick={(e) => {
                                e.stopPropagation();
                                verifyKycMutation.mutate(cust.id);
                              }}
                            >
                              <ShieldCheck className="mr-1 h-3 w-3" />
                              {verifyKycMutation.isPending && verifyKycMutation.variables === cust.id
                                ? "Verifying..."
                                : "Verify KYC"}
                            </Button>
                          )}
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline"
                          className={`text-[10px] font-semibold ${statusColors[cust.status] ?? "bg-muted text-muted-foreground border-border"}`}>
                          {cust.status}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {cust.createdAt ? new Date(cust.createdAt).toLocaleDateString() : "—"}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Pagination */}
        {!isSearching && !loading && !error && (
          <div className="flex items-center justify-between px-1">
            <p className="text-xs text-muted-foreground">
              {totalElements} customer{totalElements !== 1 ? "s" : ""} total
              {statusFilter !== "ALL" && ` (filtered: ${customers.length})`}
            </p>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" className="text-xs h-7 w-7 p-0"
                disabled={page === 0}
                onClick={() => setPage((p) => Math.max(0, p - 1))}>
                <ChevronLeft className="h-3.5 w-3.5" />
              </Button>
              <span className="text-xs text-muted-foreground">
                Page {page + 1} of {Math.max(1, totalPages)}
              </span>
              <Button variant="outline" size="sm" className="text-xs h-7 w-7 p-0"
                disabled={page + 1 >= totalPages}
                onClick={() => setPage((p) => p + 1)}>
                <ChevronRight className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
        )}
      </div>
    </DashboardLayout>
  );
};

export default BorrowersPage;
