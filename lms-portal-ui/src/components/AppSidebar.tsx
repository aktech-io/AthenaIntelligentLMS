import { NemoMark } from "@/components/NemoMark";
import { useState } from "react";
import { useAuth } from "@/contexts/AuthContext";
import { useNavigate } from "react-router-dom";
import {
  LayoutDashboard,
  FileText,
  Wallet,
  CalendarDays,
  RefreshCw,
  Package,
  Wrench,
  FileStack,
  Users,
  ShieldCheck,
  Building2,
  CreditCard,
  PiggyBank,
  BarChart3,
  AlertTriangle,
  Phone,
  Scale,
  BookOpen,
  TrendingUp,
  PieChart as PieChartIcon,
  FileBarChart,
  Bot,
  Shield,
  AlertCircle,
  FileWarning,
  Lock,
  UserCog,
  Settings,
  Link,
  Bell,
  ChevronDown,
  Building2 as Building2Icon,
  Globe,
  DollarSign,
  CalendarDays as CalendarDaysIcon,
  Compass,
  Cog,
  Landmark,
  ClipboardList,
  LogOut,
  FolderOpen,
  Briefcase,
  Settings2,
  CheckCheck,
  ScanFace,
  FlaskConical,
  Inbox,
  Receipt,
  Anchor,
  type LucideIcon,
} from "lucide-react";
import { hasPermission } from "@/lib/permissions";
import { NavLink } from "@/components/NavLink";
import { useLocation } from "react-router-dom";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarHeader,
  SidebarFooter,
  useSidebar,
} from "@/components/ui/sidebar";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";

// requires: optional RBAC permission key — entries carrying one are hidden
// from users who don't hold it (checked against the JWT, ADMIN/MANAGER
// role fallback mirrors the backend's auth.RequirePermission).
type NavItem = { title: string; url: string; icon: LucideIcon; requires?: string };

const lendingNav = [
  { title: "Loan Applications", url: "/loans", icon: FileText },
  { title: "Active Loans", url: "/active-loans", icon: Wallet },
  { title: "Repayment Schedule", url: "/repayments", icon: CalendarDays },
  { title: "Loan Modifications", url: "/modifications", icon: RefreshCw },
];

const invoiceFinanceNav = [
  { title: "Overview", url: "/invoice-finance", icon: FileText },
  { title: "Review Queue", url: "/invoice-finance/queue", icon: Inbox },
  { title: "Invoices", url: "/invoice-finance/invoices", icon: Receipt },
  { title: "Buyers", url: "/invoice-finance/buyers", icon: Building2 },
  { title: "Policy", url: "/invoice-finance/policy", icon: Scale },
  { title: "Anchors", url: "/invoice-finance/anchors", icon: Anchor },
];

const productsNav = [
  { title: "Product Catalogue", url: "/products", icon: Package },
  { title: "Product Config Engine", url: "/product-config", icon: Wrench },
  { title: "Product Templates", url: "/templates", icon: FileStack },
];

const customersNav = [
  { title: "Customer Directory", url: "/borrowers", icon: Users },
  { title: "KYC / Verification", url: "/compliance", icon: ShieldCheck },
  { title: "Business (KYB)", url: "/kyb", icon: Building2 },
];

const accountsNav = [
  { title: "Account Directory", url: "/accounts", icon: Landmark },
  { title: "Deposit Products", url: "/deposit-products", icon: Package },
  { title: "Open New Account", url: "/account-opening", icon: FileText },
  { title: "Fixed Deposits", url: "/fixed-deposits", icon: Lock },
  { title: "Interest Accrual", url: "/interest-accrual", icon: TrendingUp },
];

const floatNav = [
  { title: "Float Overview", url: "/float", icon: CreditCard },
  { title: "Overdraft Facilities", url: "/overdraft", icon: CreditCard },
  { title: "Float Analytics", url: "/float-analytics", icon: BarChart3 },
];

const collectionsNav = [
  { title: "Delinquency Queue", url: "/collections", icon: AlertTriangle },
  { title: "Collections Workbench", url: "/collections-workbench", icon: Phone },
  { title: "Legal & Write-Offs", url: "/legal", icon: Scale },
  { title: "Strategies", url: "/collections/strategies", icon: Settings },
  { title: "Analytics", url: "/collections/analytics", icon: BarChart3 },
];

const financeNav = [
  { title: "General Ledger", url: "/ledger", icon: BookOpen },
  { title: "Income Statement", url: "/income-statement", icon: TrendingUp },
  { title: "Balance Sheet", url: "/balance-sheet", icon: PieChartIcon },
  { title: "Trial Balance", url: "/trial-balance", icon: FileBarChart },
  { title: "Cash Flow", url: "/cash-flow", icon: DollarSign },
];

const complianceNav = [
  { title: "Fraud Dashboard", url: "/fraud-dashboard", icon: Shield },
  { title: "AML Monitoring", url: "/aml", icon: AlertTriangle },
  { title: "Fraud Alerts", url: "/fraud", icon: AlertCircle },
  { title: "Investigation Cases", url: "/fraud-cases", icon: Briefcase },
  { title: "Detection Rules", url: "/fraud-rules", icon: Settings2 },
  { title: "SAR / CTR Reports", url: "/sar-reports", icon: FileWarning },
  { title: "Onboarding Referrals", url: "/onboarding-referrals", icon: ScanFace },
  // Liveness model training/red-team observability — visible only to users
  // who hold compliance.decide (same gate as the backend /api/v1/ml routes).
  { title: "Model Training", url: "/model-training", icon: FlaskConical, requires: "compliance.decide" },
  { title: "Watchlist", url: "/watchlist", icon: ShieldCheck },
  { title: "Audit Logs", url: "/audit", icon: Lock },
];

const reportsNav = [
  { title: "Portfolio Analytics", url: "/reports", icon: BarChart3 },
  { title: "Portfolio at Risk", url: "/par-report", icon: AlertTriangle },
  { title: "AI Model Performance", url: "/ai", icon: Bot },
];

const adminNav = [
  { title: "Users & Roles", url: "/users", icon: UserCog },
  { title: "Approvals", url: "/approvals", icon: CheckCheck },
  { title: "Dual-Control Settings", url: "/control-settings", icon: ShieldCheck },
  { title: "System Configuration", url: "/settings", icon: Settings },
  { title: "Integrations & API", url: "/integrations", icon: Link },
  { title: "Notifications", url: "/notifications", icon: Bell },
  { title: "Document Store", url: "/documents", icon: FolderOpen },
];

const organisationNav = [
  { title: "Branch Directory", url: "/branches", icon: Building2Icon },
  { title: "Country Entities", url: "/countries", icon: Globe },
  { title: "Currencies & FX Rates", url: "/currencies", icon: DollarSign },
  { title: "Holiday Calendars", url: "/countries", icon: CalendarDaysIcon },
];

const setupNav = [
  { title: "Setup Wizard", url: "/setup-wizard", icon: Compass },
  { title: "Institution Settings", url: "/settings", icon: Cog },
];

const tellerNav = [
  { title: "My Teller Session", url: "/teller", icon: Landmark },
  { title: "Branch Cash Summary", url: "/teller", icon: ClipboardList },
];

const navSections = [
  { label: "Lending", items: lendingNav },
  { label: "Invoice Finance", items: invoiceFinanceNav },
  { label: "Products", items: productsNav },
  { label: "Customers", items: customersNav },
  { label: "Accounts & Deposits", items: accountsNav },
  { label: "Float & Overdraft", items: floatNav },
  { label: "Collections", items: collectionsNav },
  { label: "Finance", items: financeNav },
  { label: "Compliance", items: complianceNav },
  { label: "Reports", items: reportsNav },
  { label: "Administration", items: adminNav },
  { label: "Organisation", items: organisationNav },
  { label: "Setup & Configuration", items: setupNav },
  { label: "Teller", items: tellerNav },
];

export function AppSidebar() {
  const { state } = useSidebar();
  const collapsed = state === "collapsed";
  const location = useLocation();
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  const setupComplete =
    typeof window !== "undefined" && localStorage.getItem("athena_setup_complete") === "true";

  const handleLogout = () => {
    logout();
    navigate("/login");
  };

  // Determine which sections should be open by default (ones with active route)
  const getDefaultOpen = () => {
    const openSections: Record<string, boolean> = {};
    navSections.forEach((section) => {
      const hasActiveRoute = section.items.some(
        (item) => location.pathname === item.url || (item.url !== "/" && location.pathname.startsWith(item.url + "/"))
      );
      openSections[section.label] = hasActiveRoute;
    });
    // If no section is active, open Lending by default
    if (!Object.values(openSections).some(Boolean)) {
      openSections["Lending"] = true;
    }
    return openSections;
  };

  const [openSections, setOpenSections] = useState<Record<string, boolean>>(getDefaultOpen);

  const toggleSection = (label: string) => {
    setOpenSections((prev) => ({ ...prev, [label]: !prev[label] }));
  };

  const renderItems = (items: NavItem[]) =>
    items
      // Permission-gated entries (UI affordance only — the backend enforces).
      .filter((item) => !item.requires || hasPermission(item.requires, ["ADMIN", "MANAGER"]))
      .map((item) => {
      const isSetupWizard = item.url === "/setup-wizard";
      const showWarningDot = isSetupWizard && !setupComplete;
      return (
        <SidebarMenuItem key={item.title}>
          <SidebarMenuButton asChild>
            <NavLink
              to={item.url}
              end={item.url === "/" || item.url === "/invoice-finance"}
              className="group flex items-center gap-3 px-3 py-2 rounded-lg text-sidebar-foreground hover:text-sidebar-accent-foreground hover:bg-sidebar-accent/70 transition-colors text-sm border-l-2 border-transparent [&_svg]:text-accent/80 [&_svg]:transition-colors hover:[&_svg]:text-accent"
              activeClassName="bg-sidebar-accent text-sidebar-accent-foreground font-semibold !border-primary [&_svg]:!text-primary shadow-[inset_0_1px_0_hsl(var(--nemo-glass)/0.12)]"
            >
              <div className="relative shrink-0">
                <item.icon className="h-[18px] w-[18px]" />
                {showWarningDot && (
                  <span className="absolute -top-0.5 -right-0.5 h-2 w-2 rounded-full bg-amber-400 ring-1 ring-sidebar" />
                )}
              </div>
              {!collapsed && (
                <span className="flex items-center gap-1.5">
                  {item.title}
                  {showWarningDot && (
                    <span className="h-1.5 w-1.5 rounded-full bg-amber-400 shrink-0" />
                  )}
                </span>
              )}
            </NavLink>
          </SidebarMenuButton>
        </SidebarMenuItem>
      );
    });

  return (
    <Sidebar collapsible="icon" className="border-r-0">
      <SidebarHeader className="p-4 border-b border-sidebar-border">
        <div className="flex items-center gap-3">
          <NemoMark className="h-9 w-9 shrink-0" />
          {!collapsed && (
            <div>
              <h2 className="font-heading text-sidebar-accent-foreground text-lg font-bold tracking-[0.08em] uppercase leading-none">
                Nemo
              </h2>
              <p className="text-[11px] text-sidebar-foreground/80 uppercase tracking-[0.18em] font-sans mt-1">
                Neobank in a Box
              </p>
            </div>
          )}
        </div>
      </SidebarHeader>

      <SidebarContent className="px-2 py-2 overflow-y-auto">
        {/* Dashboard - standalone */}
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton asChild>
                  <NavLink
                    to="/"
                    end
                    className="group flex items-center gap-3 px-3 py-2 rounded-lg text-sidebar-foreground hover:text-sidebar-accent-foreground hover:bg-sidebar-accent/70 transition-colors text-sm border-l-2 border-transparent [&_svg]:text-accent/80 hover:[&_svg]:text-accent"
                    activeClassName="bg-sidebar-accent text-sidebar-accent-foreground font-semibold !border-primary [&_svg]:!text-primary"
                  >
                    <LayoutDashboard className="h-[18px] w-[18px] shrink-0" />
                    {!collapsed && <span>Overview Dashboard</span>}
                  </NavLink>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        {navSections.map((section) => (
          <Collapsible
            key={section.label}
            open={collapsed ? false : openSections[section.label]}
            onOpenChange={() => !collapsed && toggleSection(section.label)}
          >
            <SidebarGroup>
              {!collapsed ? (
                <CollapsibleTrigger asChild>
                  <SidebarGroupLabel className="text-accent/90 text-[11px] uppercase tracking-[0.16em] px-3 mb-1 mt-3 font-sans font-bold cursor-pointer hover:text-accent transition-colors select-none flex items-center justify-between w-full h-auto">
                    <span>{section.label}</span>
                    <ChevronDown
                      className={`h-3.5 w-3.5 shrink-0 transition-transform duration-200 ${
                        openSections[section.label] ? "rotate-0" : "-rotate-90"
                      }`}
                    />
                  </SidebarGroupLabel>
                </CollapsibleTrigger>
              ) : (
                <SidebarGroupLabel className="text-accent/90 text-[11px] uppercase tracking-[0.16em] px-3 mb-1 mt-3 font-sans font-bold h-auto">
                  {section.label}
                </SidebarGroupLabel>
              )}
              <CollapsibleContent>
                <SidebarGroupContent>
                  <SidebarMenu>{renderItems(section.items)}</SidebarMenu>
                </SidebarGroupContent>
              </CollapsibleContent>
            </SidebarGroup>
          </Collapsible>
        ))}
      </SidebarContent>

      <SidebarFooter className="p-3 border-t border-sidebar-border">
        {!collapsed && (
          <div className="flex items-center gap-3 px-2">
            <div className="h-9 w-9 rounded-full bg-gradient-to-br from-primary to-[hsl(var(--nemo-coral-deep))] flex items-center justify-center text-xs font-bold text-white font-sans shadow-glow">
              {user?.initials || "??"}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-semibold text-sidebar-accent-foreground truncate font-sans">{user?.name || "Guest"}</p>
              <p className="text-xs text-sidebar-foreground/90 truncate font-sans">{user?.role || ""}</p>
            </div>
            <button onClick={handleLogout} className="p-1.5 rounded-md hover:bg-sidebar-accent transition-colors" title="Log out">
              <LogOut className="h-[18px] w-[18px] text-sidebar-foreground/80 hover:text-destructive" />
            </button>
          </div>
        )}
      </SidebarFooter>
    </Sidebar>
  );
}
