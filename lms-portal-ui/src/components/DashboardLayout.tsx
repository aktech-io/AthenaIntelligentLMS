import { useState } from "react";
import { useAuth } from "@/contexts/AuthContext";
import { SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/AppSidebar";
import { Bell, Search, Globe, ChevronRight } from "lucide-react";
import { Input } from "@/components/ui/input";
import { useLocation, Link } from "react-router-dom";
import { BranchSwitcher, type BranchSelection } from "@/components/BranchSwitcher";
import { ThemeToggle } from "@/components/ThemeToggle";

interface DashboardLayoutProps {
  children: React.ReactNode;
  title: string;
  subtitle?: string;
  breadcrumbs?: { label: string; href?: string }[];
}

const routeLabels: Record<string, string> = {
  "/": "Dashboard",
  "/loans": "Loan Applications",
  "/active-loans": "Active Loans",
  "/borrowers": "Customer Directory",
  "/collections": "Delinquency Queue",
  "/reports": "Portfolio Analytics",
  "/settings": "System Configuration",
  "/accounts": "Accounts",
  "/transactions": "Transactions",
  "/ledger": "General Ledger",
  "/compliance": "KYC / Verification",
  "/ai": "AI Model Performance",
  "/invoice-finance": "Invoice Finance",
  "/invoice-finance/queue": "Review Queue",
  "/invoice-finance/invoices": "Invoices",
  "/invoice-finance/buyers": "Buyers",
  "/invoice-finance/policy": "Advance Policy",
  "/invoice-finance/anchors": "Anchors",
};

export function DashboardLayout({ children, title, subtitle, breadcrumbs }: DashboardLayoutProps) {
  const { user } = useAuth();
  const location = useLocation();
  const [currentBranch, setCurrentBranch] = useState<BranchSelection>({
    id: "br-001", name: "Nairobi HQ", currency: "KES", countryFlag: "🇰🇪",
  });
  const currentLabel = routeLabels[location.pathname] || title;

  const defaultBreadcrumbs = [
    { label: "Home", href: "/" },
    { label: currentLabel },
  ];
  const crumbs = breadcrumbs || defaultBreadcrumbs;

  return (
    <SidebarProvider>
      <div className="min-h-screen flex w-full">
        <AppSidebar />
        <div className="flex-1 flex flex-col min-w-0">
          {/* Top bar — 64px */}
          <header className="h-16 border-b bg-card flex items-center justify-between px-5 shrink-0">
            <div className="flex items-center gap-3">
              <SidebarTrigger className="text-muted-foreground hover:text-foreground" />
              <div className="hidden sm:flex items-center gap-1.5 text-xs text-muted-foreground">
                {crumbs.map((crumb, i) => (
                  <span key={i} className="flex items-center gap-1.5">
                    {i > 0 && <ChevronRight className="h-3 w-3" />}
                    {crumb.href ? (
                      <Link to={crumb.href} className="hover:text-foreground transition-colors">{crumb.label}</Link>
                    ) : (
                      <span className="text-foreground font-medium">{crumb.label}</span>
                    )}
                  </span>
                ))}
              </div>
            </div>
            <div className="flex items-center gap-3">
              <BranchSwitcher currentBranch={currentBranch} onBranchChange={setCurrentBranch} />
              <div className="relative hidden md:block">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
                <Input
                  placeholder="Search customers, loans, applications..."
                  className="pl-9 h-9 w-72 text-xs bg-muted/50 border-0 font-sans"
                />
              </div>
              <button className="p-2 rounded-md hover:bg-muted transition-colors hidden sm:block">
                <Globe className="h-4 w-4 text-muted-foreground" />
              </button>
              <ThemeToggle />
              <button className="relative p-2 rounded-md hover:bg-muted transition-colors">
                <Bell className="h-4 w-4 text-muted-foreground" />
                <span className="absolute top-1 right-1 h-2 w-2 rounded-full bg-destructive" />
              </button>
              <div className="h-8 w-8 rounded-full bg-primary flex items-center justify-center text-xs font-medium text-primary-foreground font-sans ml-1">
                {user?.initials || "??"}
              </div>
            </div>
          </header>

          {/* Page header */}
          <div className="px-5 pt-5 pb-3">
            <h1 className="font-heading text-xl">{title}</h1>
            {subtitle && <p className="text-sm text-muted-foreground font-sans mt-0.5">{subtitle}</p>}
          </div>

          {/* Content */}
          <main className="flex-1 overflow-auto px-5 pb-6">
            {children}
          </main>
        </div>
      </div>
    </SidebarProvider>
  );
}
