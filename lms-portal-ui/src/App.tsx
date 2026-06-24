import { Toaster } from "@/components/ui/toaster";
import { Toaster as Sonner } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route, useLocation, Link } from "react-router-dom";
import { AuthProvider } from "@/contexts/AuthContext";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { useAuth } from "@/contexts/AuthContext";
import LoginPage from "./pages/LoginPage";
import Index from "./pages/Index";
import NotFound from "./pages/NotFound";
import LoansPage from "./pages/LoansPage";
import BorrowersPage from "./pages/BorrowersPage";
import CollectionsPage from "./pages/CollectionsPage";
import ReportsPage from "./pages/ReportsPage";
import SettingsPage from "./pages/SettingsPage";
import AccountsPage from "./pages/AccountsPage";
import TransactionsPage from "./pages/TransactionsPage";
import LedgerPage from "./pages/LedgerPage";
import CompliancePage from "./pages/CompliancePage";
import AIPage from "./pages/AIPage";
import ProductsPage from "./pages/ProductsPage";
import ProductConfigPage from "./pages/ProductConfigPage";
import ProductTemplatesPage from "./pages/ProductTemplatesPage";
import ActiveLoansPage from "./pages/ActiveLoansPage";
import LoanDetailPage from "./pages/LoanDetailPage";
import Customer360Page from "./pages/Customer360Page";
import CollectionsWorkbenchPage from "./pages/CollectionsWorkbenchPage";
import IncomeStatementPage from "./pages/IncomeStatementPage";
import BalanceSheetPage from "./pages/BalanceSheetPage";
import TrialBalancePage from "./pages/TrialBalancePage";
import AMLPage from "./pages/AMLPage";
import FraudAlertsPage from "./pages/FraudAlertsPage";
import FraudDashboardPage from "./pages/FraudDashboardPage";
import FraudCasesPage from "./pages/FraudCasesPage";
import FraudRulesPage from "./pages/FraudRulesPage";
import SARReportsPage from "./pages/SARReportsPage";
import WatchlistPage from "./pages/WatchlistPage";
import AuditLogsPage from "./pages/AuditLogsPage";
import FloatPage from "./pages/FloatPage";
import WalletsPage from "./pages/WalletsPage";
import OverdraftManagementPage from "./pages/OverdraftManagementPage";
import FloatAnalyticsPage from "./pages/FloatAnalyticsPage";
import AccountDetailPage from "./pages/AccountDetailPage";
import AccountOpeningPage from "./pages/AccountOpeningPage";
import DepositProductsPage from "./pages/DepositProductsPage";
import FixedDepositsPage from "./pages/FixedDepositsPage";
import InterestAccrualPage from "./pages/InterestAccrualPage";
import UsersPage from "./pages/UsersPage";
import IntegrationsPage from "./pages/IntegrationsPage";
import NotificationsPage from "./pages/NotificationsPage";
import LegalPage from "./pages/LegalPage";
import CaseDetailPage from "./pages/CaseDetailPage";
import CollectionStrategiesPage from "./pages/CollectionStrategiesPage";
import CollectionsAnalyticsPage from "./pages/CollectionsAnalyticsPage";
import RepaymentsPage from "./pages/RepaymentsPage";
import ModificationsPage from "./pages/ModificationsPage";
import KYBPage from "./pages/KYBPage";
import SetupWizardPage from "./pages/SetupWizardPage";
import BranchDirectoryPage from "./pages/BranchDirectoryPage";
import CountryConfigPage from "./pages/CountryConfigPage";
import CurrenciesFxPage from "./pages/CurrenciesFxPage";
import TellerSessionPage from "./pages/TellerSessionPage";
import ConsolidatedReportsPage from "./pages/ConsolidatedReportsPage";
import DocumentsPage from "./pages/DocumentsPage";
import CashFlowPage from "./pages/CashFlowPage";
import ApprovalsPage from "./pages/ApprovalsPage";
import ControlSettingsPage from "./pages/ControlSettingsPage";
import { ErrorBoundary } from "@/components/ErrorBoundary";

const queryClient = new QueryClient();

/** Banner shown on the dashboard when setup is not yet complete */
function SetupBanner() {
  const { isAuthenticated } = useAuth();
  const location = useLocation();
  const setupComplete =
    typeof window !== "undefined" && localStorage.getItem("athena_setup_complete") === "true";

  if (!isAuthenticated || setupComplete || location.pathname === "/setup-wizard") {
    return null;
  }

  // Only show on the dashboard root
  if (location.pathname !== "/") return null;

  return (
    <div className="bg-amber-50 border-b border-amber-200 px-4 py-2 flex items-center justify-between text-sm text-amber-800">
      <span>
        Setup not complete. Configure your institution first.
      </span>
      <Link
        to="/setup-wizard"
        className="ml-4 underline font-medium hover:text-amber-900 shrink-0"
      >
        Go to Setup Wizard
      </Link>
    </div>
  );
}

const P = ({ children }: { children: React.ReactNode }) => (
  <ProtectedRoute>{children}</ProtectedRoute>
);

const AppRoutes = () => (
  <ErrorBoundary>
    <SetupBanner />
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/" element={<P><Index /></P>} />
      <Route path="/loans" element={<P><LoansPage /></P>} />
      <Route path="/borrowers" element={<P><BorrowersPage /></P>} />
      <Route path="/collections" element={<P><CollectionsPage /></P>} />
      <Route path="/reports" element={<P><ReportsPage /></P>} />
      <Route path="/settings" element={<P><SettingsPage /></P>} />
      <Route path="/accounts" element={<P><AccountsPage /></P>} />
      <Route path="/account/:accountId" element={<P><AccountDetailPage /></P>} />
      <Route path="/account-opening" element={<P><AccountOpeningPage /></P>} />
      <Route path="/deposit-products" element={<P><DepositProductsPage /></P>} />
      <Route path="/fixed-deposits" element={<P><FixedDepositsPage /></P>} />
      <Route path="/interest-accrual" element={<P><InterestAccrualPage /></P>} />
      <Route path="/transactions" element={<P><TransactionsPage /></P>} />
      <Route path="/ledger" element={<P><LedgerPage /></P>} />
      <Route path="/compliance" element={<P><CompliancePage /></P>} />
      <Route path="/ai" element={<P><AIPage /></P>} />
      <Route path="/products" element={<P><ProductsPage /></P>} />
      <Route path="/product-config" element={<P><ProductConfigPage /></P>} />
      <Route path="/templates" element={<P><ProductTemplatesPage /></P>} />
      <Route path="/active-loans" element={<P><ActiveLoansPage /></P>} />
      <Route path="/loan/:loanId" element={<P><LoanDetailPage /></P>} />
      <Route path="/customer/:customerId" element={<P><Customer360Page /></P>} />
      <Route path="/collections-workbench" element={<P><CollectionsWorkbenchPage /></P>} />
      <Route path="/collections/case/:id" element={<P><CaseDetailPage /></P>} />
      <Route path="/collections/strategies" element={<P><CollectionStrategiesPage /></P>} />
      <Route path="/collections/analytics" element={<P><CollectionsAnalyticsPage /></P>} />
      <Route path="/income-statement" element={<P><IncomeStatementPage /></P>} />
      <Route path="/balance-sheet" element={<P><BalanceSheetPage /></P>} />
      <Route path="/trial-balance" element={<P><TrialBalancePage /></P>} />
      <Route path="/cash-flow" element={<P><CashFlowPage /></P>} />
      <Route path="/aml" element={<P><AMLPage /></P>} />
      <Route path="/fraud" element={<P><FraudAlertsPage /></P>} />
      <Route path="/fraud-dashboard" element={<P><FraudDashboardPage /></P>} />
      <Route path="/fraud-cases" element={<P><FraudCasesPage /></P>} />
      <Route path="/fraud-rules" element={<P><FraudRulesPage /></P>} />
      <Route path="/sar-reports" element={<P><SARReportsPage /></P>} />
      <Route path="/watchlist" element={<P><WatchlistPage /></P>} />
      <Route path="/audit" element={<P><AuditLogsPage /></P>} />
      <Route path="/float" element={<P><FloatPage /></P>} />
      <Route path="/wallets" element={<P><WalletsPage /></P>} />
      <Route path="/overdraft" element={<P><OverdraftManagementPage /></P>} />
      <Route path="/float-analytics" element={<P><FloatAnalyticsPage /></P>} />
      <Route path="/users" element={<P><UsersPage /></P>} />
      <Route path="/integrations" element={<P><IntegrationsPage /></P>} />
      <Route path="/notifications" element={<P><NotificationsPage /></P>} />
      <Route path="/legal" element={<P><LegalPage /></P>} />
      <Route path="/repayments" element={<P><RepaymentsPage /></P>} />
      <Route path="/modifications" element={<P><ModificationsPage /></P>} />
      <Route path="/kyb" element={<P><KYBPage /></P>} />
      <Route path="/setup-wizard" element={<P><SetupWizardPage /></P>} />
      <Route path="/branches" element={<P><BranchDirectoryPage /></P>} />
      <Route path="/countries" element={<P><CountryConfigPage /></P>} />
      <Route path="/currencies" element={<P><CurrenciesFxPage /></P>} />
      <Route path="/teller" element={<P><TellerSessionPage /></P>} />
      <Route path="/consolidated-reports" element={<P><ConsolidatedReportsPage /></P>} />
      <Route path="/documents" element={<P><DocumentsPage /></P>} />
      <Route path="/approvals" element={<P><ApprovalsPage /></P>} />
      <Route path="/control-settings" element={<P><ControlSettingsPage /></P>} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  </ErrorBoundary>
);

const App = () => (
  <QueryClientProvider client={queryClient}>
    <TooltipProvider>
      <Toaster />
      <Sonner />
      <BrowserRouter>
        <AuthProvider>
          <AppRoutes />
        </AuthProvider>
      </BrowserRouter>
    </TooltipProvider>
  </QueryClientProvider>
);

export default App;
