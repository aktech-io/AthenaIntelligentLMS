import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { RefreshCw, TrendingUp, CheckCircle } from "lucide-react";

const ModificationsPage = () => {
  return (
    <DashboardLayout
      title="Loan Modifications"
      subtitle="Reschedules, top-ups, and rate changes"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Lending" }, { label: "Loan Modifications" }]}
    >
      <div className="space-y-6 animate-fade-in">
        <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Total Requests</span>
                <RefreshCw className="h-4 w-4 text-muted-foreground" />
              </div>
              <p className="text-2xl font-heading">0</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Pending</span>
                <RefreshCw className="h-4 w-4 text-warning" />
              </div>
              <p className="text-2xl font-heading">0</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Approved</span>
                <CheckCircle className="h-4 w-4 text-success" />
              </div>
              <p className="text-2xl font-heading">0</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Top-Ups</span>
                <TrendingUp className="h-4 w-4 text-accent" />
              </div>
              <p className="text-2xl font-heading">0</p>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium">Modification Requests</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
              <RefreshCw className="h-8 w-8 mb-2 text-muted-foreground/50" />
              <p className="text-sm font-medium">No modification requests</p>
              <p className="text-xs mt-1">Loan modification requests (reschedules, top-ups, rate changes) will appear here.</p>
              <p className="text-[11px] mt-3 text-muted-foreground/70">
                Modifications can be initiated from the Loan Detail page for active loans.
              </p>
            </div>
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
};

export default ModificationsPage;
