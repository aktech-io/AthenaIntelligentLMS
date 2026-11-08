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
          <header className="h-16 border-b border-border/70 bg-card/80 backdrop-blur-md flex items-center justify-between px-6 shrink-0 sticky top-0 z-20">
            <div className="flex items-center gap-3">
              <SidebarTrigger className="text-muted-foreground hover:text-foreground" />
              <div className="hidden sm:flex items-center gap-1.5 text-sm text-muted-foreground">
                {crumbs.map((crumb, i) => (
                  <span key={i} className="flex items-center gap-1.5">
                    {i > 0 && <ChevronRight className="h-3.5 w-3.5 text-accent/70" />}
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
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-accent/80" />
                <Input
                  placeholder="Search customers, loans, applications…"
                  className="pl-9 h-10 w-80 text-sm bg-muted/60 border border-border/60 focus-visible:border-accent font-sans rounded-lg"
                />
              </div>
              <button className="p-2 rounded-md hover:bg-muted transition-colors hidden sm:block">
                <Globe className="h-[18px] w-[18px] text-muted-foreground" />
              </button>
              <ThemeToggle />
              <button className="relative p-2 rounded-md hover:bg-muted transition-colors">
                <Bell className="h-[18px] w-[18px] text-muted-foreground" />
                <span className="absolute top-1.5 right-1.5 h-2 w-2 rounded-full bg-primary ring-2 ring-card" />
              </button>
              <div className="h-9 w-9 rounded-full bg-gradient-to-br from-primary to-[hsl(var(--nemo-coral-deep))] flex items-center justify-center text-xs font-bold text-primary-foreground font-sans ml-1 shadow-glow">
                {user?.initials || "??"}
              </div>
            </div>
          </header>

          {/* Page header */}
          <div className="px-6 pt-7 pb-4">
            <h1 className="font-heading text-3xl font-semibold tracking-tight">{title}</h1>
            {subtitle && <p className="text-base text-muted-foreground font-sans mt-1 max-w-3xl">{subtitle}</p>}
          </div>

          {/* Content */}
          <main className="flex-1 overflow-auto px-6 pb-10">
            {children}
          </main>
        </div>
      </div>
    </SidebarProvider>
  );
}
