import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Network } from "lucide-react";

const integrations = [
  {
    name: "Payment Gateway",
    description: "M-Pesa / Mobile Money",
    path: "/proxy/payment",
    target: "payment-service:8090",
    status: "Active",
  },
  {
    name: "Loan Origination",
    description: "Application Processing",
    path: "/proxy/origination",
    target: "loan-origination:8088",
    status: "Active",
  },
  {
    name: "Loan Management",
    description: "Portfolio Management",
    path: "/proxy/management",
    target: "loan-management:8089",
    status: "Active",
  },
  {
    name: "Accounting",
    description: "GL Journal Entries",
    path: "/proxy/accounting",
    target: "accounting-service:8091",
    status: "Active",
  },
  {
    name: "Reporting",
    description: "Portfolio Analytics",
    path: "/proxy/reporting",
    target: "reporting-service:8095",
    status: "Active",
  },
  {
    name: "AI Credit Scoring",
    description: "Risk Assessment",
    path: "/proxy/scoring",
    target: "ai-scoring-service:8096",
    status: "Active",
  },
  {
    name: "Float Management",
    description: "Liquidity Control",
    path: "/proxy/float",
    target: "float-service:8092",
    status: "Active",
  },
  {
    name: "Compliance",
    description: "Regulatory Monitoring",
    path: "/proxy/compliance",
    target: "compliance-service:8094",
    status: "Active",
  },
  {
    name: "Collections",
    description: "Debt Recovery",
    path: "/proxy/collections",
    target: "collections-service:8093",
    status: "Active",
  },
];

const IntegrationsPage = () => {
  return (
    <DashboardLayout
      title="Integrations & API"
      subtitle="Service mesh configuration"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Administration" }, { label: "Integrations" }]}
    >
      <div className="space-y-6 animate-fade-in">
        {/* Summary */}
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Total Integrations</span>
                <Network className="h-4 w-4 text-muted-foreground" />
              </div>
              <p className="text-2xl font-heading">{integrations.length}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Active</span>
                <Network className="h-4 w-4 text-success" />
              </div>
              <p className="text-2xl font-heading text-success">
                {integrations.filter((i) => i.status === "Active").length}
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Gateway</span>
              </div>
              <p className="text-sm font-medium font-mono mt-1">nginx reverse proxy</p>
              <p className="text-[11px] text-muted-foreground">lms-portal-ui:3001</p>
            </CardContent>
          </Card>
        </div>

        {/* Integrations table */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium">Configured Service Integrations</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="text-xs">Integration</TableHead>
                  <TableHead className="text-xs">Description</TableHead>
                  <TableHead className="text-xs">Proxy Path</TableHead>
                  <TableHead className="text-xs">Upstream Target</TableHead>
                  <TableHead className="text-xs">Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {integrations.map((intg) => (
                  <TableRow key={intg.path}>
                    <TableCell className="text-sm font-medium">{intg.name}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">{intg.description}</TableCell>
                    <TableCell className="text-xs font-mono">{intg.path}</TableCell>
                    <TableCell className="text-xs font-mono text-muted-foreground">{intg.target}</TableCell>
                    <TableCell>
                      <Badge
                        variant="outline"
                        className="text-[11px] bg-success/10 text-success border-success/20"
                      >
                        {intg.status}
                      </Badge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
};

export default IntegrationsPage;
