import { Routes, Route, Navigate, useParams } from "react-router-dom";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import SellerLoginPage from "./SellerLoginPage";
import SellerDashboardPage from "./SellerDashboardPage";
import SellerUploadPage from "./SellerUploadPage";
import SellerInvoicesPage from "./SellerInvoicesPage";
import SellerInvoiceDetailPage from "./SellerInvoiceDetailPage";
import SellerBuyersPage from "./SellerBuyersPage";
import SellerProfilePage from "./SellerProfilePage";

const P = ({ children }: { children: React.ReactNode }) => <ProtectedRoute>{children}</ProtectedRoute>;

/**
 * APP_MODE=invoice-client — the standalone seller app (mobile-first).
 * Login is phone + OTP via bff-gateway; the JWT is stored under `athena_jwt`
 * so ProtectedRoute/AuthContext and invoiceService work unchanged.
 */
export default function SellerApp({ publicRoutes }: { publicRoutes?: React.ReactNode }) {
  return (
    <Routes>
      <Route path="/login" element={<SellerLoginPage />} />
      <Route path="/" element={<P><SellerDashboardPage /></P>} />
      <Route path="/upload" element={<P><SellerUploadPage /></P>} />
      <Route path="/invoices" element={<P><SellerInvoicesPage /></P>} />
      <Route path="/invoices/:id" element={<P><SellerInvoiceDetailPage /></P>} />
      <Route path="/buyers" element={<P><SellerBuyersPage /></P>} />
      <Route path="/profile" element={<P><SellerProfilePage /></P>} />
      {/* portal-style deep links → seller equivalents */}
      <Route path="/invoice-finance/invoices/:id" element={<RedirectInvoice />} />
      <Route path="/invoice-finance/*" element={<Navigate to="/" replace />} />
      {publicRoutes}
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

function RedirectInvoice() {
  const { id = "" } = useParams();
  return <Navigate to={`/invoices/${id}`} replace />;
}
