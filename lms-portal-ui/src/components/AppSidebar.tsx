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
} from "lucide-react";
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

const lendingNav = [
  { title: "Loan Applications", url: "/loans", icon: FileText },
  { title: "Active Loans", url: "/active-loans", icon: Wallet },
  { title: "Repayment Schedule", url: "/repayments", icon: CalendarDays },
  { title: "Loan Modifications", url: "/modifications", icon: RefreshCw },
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
        (item) => location.pathname === item.url
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

  const renderItems = (items: typeof lendingNav) =>
    items.map((item) => {
      const isSetupWizard = item.url === "/setup-wizard";
      const showWarningDot = isSetupWizard && !setupComplete;
      return (
        <SidebarMenuItem key={item.title}>
          <SidebarMenuButton asChild>
            <NavLink
              to={item.url}
              end={item.url === "/"}
              className="flex items-center gap-3 px-3 py-1.5 rounded-md text-sidebar-foreground/70 hover:text-sidebar-accent-foreground hover:bg-sidebar-accent transition-colors text-[13px]"
              activeClassName="bg-sidebar-accent text-sidebar-accent-foreground font-medium"
            >
              <div className="relative shrink-0">
                <item.icon className="h-4 w-4" />
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
          <div className="h-9 w-9 rounded-lg bg-accent flex items-center justify-center shrink-0">
            <span className="font-heading font-bold text-accent-foreground text-base">A</span>
          </div>
          {!collapsed && (
            <div>
              <h2 className="font-heading text-sidebar-accent-foreground text-base tracking-wide">
                AthenaLMS
              </h2>
              <p className="text-[10px] text-sidebar-foreground/50 uppercase tracking-widest font-sans">
                Lending Platform
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
                    className="flex items-center gap-3 px-3 py-1.5 rounded-md text-sidebar-foreground/70 hover:text-sidebar-accent-foreground hover:bg-sidebar-accent transition-colors text-[13px]"
                    activeClassName="bg-sidebar-accent text-sidebar-accent-foreground font-medium"
                  >
                    <LayoutDashboard className="h-4 w-4 shrink-0" />
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
                  <SidebarGroupLabel className="text-sidebar-foreground/40 text-[9px] uppercase tracking-[0.15em] px-3 mb-0.5 mt-1 font-sans font-semibold cursor-pointer hover:text-sidebar-foreground/70 transition-colors select-none flex items-center justify-between w-full">
                    <span>{section.label}</span>
                    <ChevronDown
                      className={`h-3 w-3 shrink-0 transition-transform duration-200 ${
                        openSections[section.label] ? "rotate-0" : "-rotate-90"
                      }`}
                    />
                  </SidebarGroupLabel>
                </CollapsibleTrigger>
              ) : (
                <SidebarGroupLabel className="text-sidebar-foreground/40 text-[9px] uppercase tracking-[0.15em] px-3 mb-0.5 mt-1 font-sans font-semibold">
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
            <div className="h-8 w-8 rounded-full bg-sidebar-accent flex items-center justify-center text-xs font-medium text-sidebar-accent-foreground font-sans">
              {user?.initials || "??"}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-xs font-medium text-sidebar-accent-foreground truncate font-sans">{user?.name || "Guest"}</p>
              <p className="text-[10px] text-sidebar-foreground/50 truncate font-sans">{user?.role || ""}</p>
            </div>
            <button onClick={handleLogout} className="p-1.5 rounded-md hover:bg-sidebar-accent transition-colors" title="Log out">
              <LogOut className="h-4 w-4 text-sidebar-foreground/50 hover:text-destructive" />
            </button>
          </div>
        )}
      </SidebarFooter>
    </Sidebar>
  );
}
