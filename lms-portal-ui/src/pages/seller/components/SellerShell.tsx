import { Link, NavLink, useLocation } from "react-router-dom";
import { Home, Upload, Receipt, Building2, UserRound, ChevronLeft, type LucideIcon } from "lucide-react";
import { NemoMark } from "@/components/NemoMark";
import { ThemeToggle } from "@/components/ThemeToggle";
import { cn } from "@/lib/utils";
import { appConfig } from "@/lib/appConfig";

type Tab = { to: string; label: string; icon: LucideIcon; end?: boolean };

const TABS: Tab[] = [
  { to: "/", label: "Home", icon: Home, end: true },
  { to: "/upload", label: "Finance", icon: Upload },
  { to: "/invoices", label: "Invoices", icon: Receipt },
  { to: "/buyers", label: "Buyers", icon: Building2 },
  { to: "/profile", label: "Profile", icon: UserRound },
];

interface Props {
  children: React.ReactNode;
  /** Page heading shown under the app bar. */
  title?: string;
  subtitle?: string;
  /** Back link target (mobile shows a chevron in the app bar). */
  back?: string;
  /** Right-aligned action in the page header. */
  action?: React.ReactNode;
  /** Widen the content column (detail pages). */
  wide?: boolean;
}

/**
 * Seller app chrome: top app bar (NemoMark + brand), bottom tab bar on mobile,
 * left rail on md+. Works at 390px and desktop. No admin sidebar.
 */
export function SellerShell({ children, title, subtitle, back, action, wide }: Props) {
  const location = useLocation();

  return (
    <div className="min-h-screen bg-background text-foreground flex flex-col md:flex-row">
      {/* Desktop left rail */}
      <aside className="hidden md:flex md:w-60 lg:w-64 shrink-0 flex-col border-r border-border/70 bg-sidebar text-sidebar-foreground sticky top-0 h-screen">
        <div className="p-4 flex items-center gap-3 border-b border-sidebar-border">
          <NemoMark className="h-9 w-9 shrink-0" />
          <div className="min-w-0">
            <p className="font-heading text-sm font-bold text-sidebar-accent-foreground leading-none truncate">Nemo Invoice Finance</p>
            <p className="text-[11px] uppercase tracking-[0.18em] text-sidebar-foreground/80 mt-1">Seller app</p>
          </div>
        </div>
        <nav className="flex-1 px-2 py-3 space-y-0.5">
          {TABS.map((t) => (
            <NavLink
              key={t.to}
              to={t.to}
              end={t.end}
              className={({ isActive }) =>
                cn(
                  "group flex items-center gap-3 px-3 py-2 rounded-lg text-sm border-l-2 border-transparent transition-colors",
                  "text-sidebar-foreground hover:text-sidebar-accent-foreground hover:bg-sidebar-accent/70 [&_svg]:text-accent/80 hover:[&_svg]:text-accent",
                  isActive && "bg-sidebar-accent text-sidebar-accent-foreground font-semibold !border-primary [&_svg]:!text-primary shadow-[inset_0_1px_0_hsl(var(--nemo-glass)/0.12)]"
                )
              }
            >
              <t.icon className="h-[18px] w-[18px]" />
              <span>{t.to === "/upload" ? "Finance an invoice" : t.label}</span>
            </NavLink>
          ))}
        </nav>
        <div className="p-3 border-t border-sidebar-border flex items-center justify-between">
          <span className="text-[11px] text-sidebar-foreground/70">
            {appConfig.demo ? "Demo environment" : "Tamper-evident ledger"}
          </span>
          <ThemeToggle />
        </div>
      </aside>

      <div className="flex-1 flex flex-col min-w-0">
        {/* Top app bar */}
        <header className="h-14 md:h-16 border-b border-border/70 bg-card/80 backdrop-blur-md sticky top-0 z-20 flex items-center justify-between px-4 md:px-6">
          <div className="flex items-center gap-3 min-w-0">
            {back ? (
              <Link to={back} className="md:hidden -ml-1 p-1.5 rounded-md hover:bg-muted text-muted-foreground" aria-label="Back">
                <ChevronLeft className="h-5 w-5" />
              </Link>
            ) : (
              <NemoMark className="h-8 w-8 md:hidden" />
            )}
            <div className="min-w-0 md:hidden">
              <p className="font-heading text-sm font-semibold leading-none truncate">{back && title ? title : "Nemo Invoice Finance"}</p>
              {!back && <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground mt-0.5">Seller app</p>}
            </div>
            <div className="hidden md:flex items-center gap-1.5 text-sm text-muted-foreground">
              {back && (
                <Link to={back} className="hover:text-foreground inline-flex items-center gap-1">
                  <ChevronLeft className="h-4 w-4" /> Back
                </Link>
              )}
              {!back && <span className="text-foreground font-medium">{title ?? TABS.find((t) => (t.end ? location.pathname === t.to : location.pathname.startsWith(t.to)))?.label}</span>}
            </div>
          </div>
          <div className="flex items-center gap-1">
            {appConfig.demo && (
              <span className="hidden sm:inline-flex items-center rounded-full border border-warning/40 bg-warning/10 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wider text-warning">
                Demo
              </span>
            )}
            <div className="md:hidden">
              <ThemeToggle />
            </div>
          </div>
        </header>

        {/* Page header */}
        {(title || action) && (
          <div className={cn("px-4 md:px-6 pt-5 md:pt-7 pb-2 w-full mx-auto flex items-start justify-between gap-3", wide ? "max-w-6xl" : "max-w-3xl")}>
            <div className="min-w-0">
              {title && <h1 className="font-heading text-2xl md:text-3xl font-semibold tracking-tight">{title}</h1>}
              {subtitle && <p className="text-sm md:text-base text-muted-foreground mt-1">{subtitle}</p>}
            </div>
            {action && <div className="shrink-0">{action}</div>}
          </div>
        )}

        {/* Content; bottom padding clears the mobile tab bar */}
        <main className={cn("flex-1 w-full mx-auto px-4 md:px-6 pb-24 md:pb-10 pt-3", wide ? "max-w-6xl" : "max-w-3xl")}>{children}</main>

        {/* Mobile bottom tab bar */}
        <nav className="md:hidden fixed bottom-0 inset-x-0 z-30 border-t border-border/70 bg-card/95 backdrop-blur-md pb-[env(safe-area-inset-bottom)]">
          <ul className="grid grid-cols-5">
            {TABS.map((t) => (
              <li key={t.to}>
                <NavLink
                  to={t.to}
                  end={t.end}
                  className={({ isActive }) =>
                    cn(
                      "flex flex-col items-center justify-center gap-1 py-2 text-[11px] font-medium transition-colors min-h-[56px]",
                      isActive ? "text-primary" : "text-muted-foreground hover:text-foreground"
                    )
                  }
                >
                  {({ isActive }) => (
                    <>
                      <span className={cn("flex items-center justify-center h-7 w-12 rounded-full transition-colors", isActive && "bg-primary/15")}>
                        <t.icon className="h-5 w-5" />
                      </span>
                      {t.label}
                    </>
                  )}
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>
      </div>
    </div>
  );
}
