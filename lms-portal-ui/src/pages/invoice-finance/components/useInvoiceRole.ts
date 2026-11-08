import { useAuth } from "@/contexts/AuthContext";
import { hasPermission } from "@/lib/permissions";

/**
 * Role affordances for invoice-finance actions (UI only — the backend enforces).
 * `user.role` may be a backend role (ADMIN/MANAGER/OFFICER) or a display name
 * ("Branch Manager", "System Administrator"); both are handled.
 */
export function useInvoiceRole() {
  const { user } = useAuth();
  const role = (user?.role ?? "").toUpperCase();
  const isAdmin = role.includes("ADMIN") || hasPermission("invoice.policy", ["ADMIN"]);
  const isManager = isAdmin || role.includes("MANAGER") || hasPermission("invoice.approve", ["ADMIN", "MANAGER"]);
  // OFFICER+ — anyone logged in to the portal qualifies, tellers excluded.
  const isOfficer = isManager || !role.includes("TELLER");
  return { isAdmin, isManager, isOfficer, role };
}
