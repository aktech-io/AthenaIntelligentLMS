import { useMemo } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Building2, CheckCircle, Clock, Users } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { customerService, type Customer } from "@/services/customerService";

const statusStyle: Record<string, string> = {
  ACTIVE: "bg-success/10 text-success border-success/20",
  PENDING: "bg-warning/10 text-warning border-warning/20",
  INACTIVE: "bg-muted text-muted-foreground border-border",
  SUSPENDED: "bg-destructive/10 text-destructive border-destructive/20",
};

const KYBPage = () => {
  const { data: apiPage, isLoading } = useQuery({
    queryKey: ["kyb-customers"],
    queryFn: () => customerService.listCustomers(0, 100),
    staleTime: 60_000,
    retry: false,
  });

  const allCustomers: Customer[] = apiPage?.content ?? [];
  const businesses = useMemo(
    () => allCustomers.filter((c) => c.customerType?.toUpperCase() === "BUSINESS"),
    [allCustomers]
  );
  const displayList = businesses.length > 0 ? businesses : allCustomers;

  const activeCount = displayList.filter((c) => c.status === "ACTIVE").length;
  const pendingCount = displayList.filter((c) => c.kycStatus === "PENDING" || c.status === "PENDING").length;

  return (
    <DashboardLayout
      title="Business (KYB)"
      subtitle="Know Your Business verification"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Customers" }, { label: "Business (KYB)" }]}
    >
      <div className="space-y-6 animate-fade-in">
        {isLoading ? (
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            {Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-20 w-full" />)}
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">
                    {businesses.length > 0 ? "Total Businesses" : "Total Customers"}
                  </span>
                  <Building2 className="h-4 w-4 text-muted-foreground" />
                </div>
                <p className="text-2xl font-heading">{displayList.length}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Active</span>
                  <CheckCircle className="h-4 w-4 text-success" />
                </div>
                <p className="text-2xl font-heading">{activeCount}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Pending KYC</span>
                  <Clock className="h-4 w-4 text-warning" />
                </div>
                <p className="text-2xl font-heading">{pendingCount}</p>
              </CardContent>
            </Card>
          </div>
        )}

        <Card>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
              </div>
            ) : displayList.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <Users className="h-8 w-8 mb-2 text-muted-foreground/50" />
                <p className="text-sm font-medium">No business customers found</p>
                <p className="text-xs mt-1">Business-type customers will appear here once registered.</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">Customer ID</TableHead>
                    <TableHead className="text-xs">Name</TableHead>
                    <TableHead className="text-xs">Email</TableHead>
                    <TableHead className="text-xs">Phone</TableHead>
                    <TableHead className="text-xs">Type</TableHead>
                    <TableHead className="text-xs">Created</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {displayList.map((c) => (
                    <TableRow key={c.id} className="table-row-hover">
                      <TableCell className="text-xs font-mono">{c.customerId}</TableCell>
                      <TableCell className="text-sm font-medium">{c.firstName} {c.lastName}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">{c.email ?? "—"}</TableCell>
                      <TableCell className="text-xs">{c.phone ?? "—"}</TableCell>
                      <TableCell className="text-xs">{c.customerType}</TableCell>
                      <TableCell className="text-xs">{c.createdAt?.split("T")[0] ?? "—"}</TableCell>
                      <TableCell>
                        <Badge variant="outline" className={`text-[11px] ${statusStyle[c.status] || ""}`}>
                          {c.status}
                        </Badge>
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

export default KYBPage;
