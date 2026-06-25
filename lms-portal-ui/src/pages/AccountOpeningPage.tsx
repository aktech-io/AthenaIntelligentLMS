import { DashboardLayout } from "@/components/DashboardLayout";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useQuery, useMutation } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { accountService } from "@/services/accountService";
import {
  depositProductService,
  type DepositProduct,
} from "@/services/depositProductService";
import { apiGet } from "@/lib/api";
import { useState } from "react";
import { useToast } from "@/hooks/use-toast";
import {
  Search,
  ChevronRight,
  ChevronLeft,
  Check,
  User,
  Package,
  FileText,
  ClipboardCheck,
  Loader2,
} from "lucide-react";
import { formatKES } from "@/lib/format";

interface Customer {
  id: string;
  firstName: string;
  lastName: string;
  email?: string;
  phoneNumber?: string;
  nationalId?: string;
  kycTier?: number;
}

const STEPS = [
  { label: "Select Customer", icon: User },
  { label: "Select Product", icon: Package },
  { label: "Account Details", icon: FileText },
  { label: "Review & Submit", icon: ClipboardCheck },
];

const categoryColors: Record<string, string> = {
  SAVINGS: "bg-green-100 text-green-800 border-green-300",
  CURRENT: "bg-blue-100 text-blue-800 border-blue-300",
  FIXED_DEPOSIT: "bg-purple-100 text-purple-800 border-purple-300",
  CALL_DEPOSIT: "bg-orange-100 text-orange-800 border-orange-300",
  WALLET: "bg-gray-100 text-gray-800 border-gray-300",
};

const AccountOpeningPage = () => {
  const navigate = useNavigate();
  const { toast } = useToast();

  const [step, setStep] = useState(0);
  const [customerSearch, setCustomerSearch] = useState("");
  const [selectedCustomer, setSelectedCustomer] = useState<Customer | null>(
    null
  );
  const [selectedProduct, setSelectedProduct] = useState<DepositProduct | null>(
    null
  );
  const [accountName, setAccountName] = useState("");
  const [currency, setCurrency] = useState("KES");
  const [branch, setBranch] = useState("");
  const [initialDeposit, setInitialDeposit] = useState("");
  const [termDays, setTermDays] = useState("");
  const [autoRenew, setAutoRenew] = useState(false);
  const [interestRateOverride, setInterestRateOverride] = useState("");

  const {
    data: customerResults,
    isFetching: searchingCustomers,
  } = useQuery({
    queryKey: ["customer-search", customerSearch],
    queryFn: async () => {
      const result = await apiGet<Customer[]>(
        `/proxy/auth/api/v1/customers/search?q=${encodeURIComponent(customerSearch)}`
      );
      if (result.error || !result.data) {
        throw new Error(result.error ?? "Search failed");
      }
      return result.data;
    },
    enabled: customerSearch.length >= 2,
    staleTime: 30_000,
  });

  const { data: productsPage, isLoading: loadingProducts } = useQuery({
    queryKey: ["deposit-products"],
    queryFn: () => depositProductService.listProducts(0, 100),
    staleTime: 60_000,
  });

  const products = productsPage?.content ?? [];

  const openAccountMutation = useMutation({
    mutationFn: () => {
      if (!selectedCustomer || !selectedProduct) {
        throw new Error("Missing customer or product");
      }
      return accountService.openAccount({
        customerId: selectedCustomer.id,
        depositProductId: selectedProduct.id,
        accountType: selectedProduct.productCategory,
        currency,
        kycTier: selectedCustomer.kycTier ?? 1,
        accountName,
        branchId: branch || undefined,
        initialDeposit: initialDeposit ? parseFloat(initialDeposit) : undefined,
        termDays:
          selectedProduct.productCategory === "FIXED_DEPOSIT" && termDays
            ? parseInt(termDays)
            : undefined,
        autoRenew:
          selectedProduct.productCategory === "FIXED_DEPOSIT"
            ? autoRenew
            : undefined,
        interestRateOverride: interestRateOverride
          ? parseFloat(interestRateOverride)
          : undefined,
      });
    },
    onSuccess: (account) => {
      toast({
        title: "Account opened successfully",
        description: `Account ${account.accountNumber} created — you can now fund it from the account page.`,
      });
      // Land on the new account so the operator can immediately fund/manage it,
      // rather than dumping them back on the list.
      navigate(`/account/${account.id}`);
    },
    onError: (err: Error) => {
      toast({
        title: "Failed to open account",
        description: err.message,
        variant: "destructive",
      });
    },
  });

  const canProceed = (): boolean => {
    switch (step) {
      case 0:
        return selectedCustomer !== null;
      case 1:
        return selectedProduct !== null;
      case 2:
        return accountName.trim().length > 0;
      case 3:
        return true;
      default:
        return false;
    }
  };

  const handleNext = () => {
    if (step < 3) setStep(step + 1);
  };

  const handleBack = () => {
    if (step > 0) setStep(step - 1);
  };

  const handleSubmit = () => {
    openAccountMutation.mutate();
  };

  const renderStepIndicator = () => (
    <div className="flex items-center justify-center mb-6">
      {STEPS.map((s, i) => {
        const Icon = s.icon;
        const isActive = i === step;
        const isComplete = i < step;
        return (
          <div key={i} className="flex items-center">
            <div
              className={`flex items-center gap-2 px-3 py-2 rounded-lg transition-all ${
                isActive
                  ? "bg-primary text-primary-foreground"
                  : isComplete
                  ? "bg-primary/10 text-primary"
                  : "bg-muted text-muted-foreground"
              }`}
            >
              {isComplete ? (
                <Check className="h-4 w-4" />
              ) : (
                <Icon className="h-4 w-4" />
              )}
              <span className="text-xs font-sans font-medium hidden sm:inline">
                {s.label}
              </span>
            </div>
            {i < STEPS.length - 1 && (
              <ChevronRight className="h-4 w-4 mx-1 text-muted-foreground" />
            )}
          </div>
        );
      })}
    </div>
  );

  const renderStep0 = () => (
    <div className="space-y-4">
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder="Search by name, email, phone, or ID..."
          className="pl-9"
          value={customerSearch}
          onChange={(e) => setCustomerSearch(e.target.value)}
        />
      </div>

      {searchingCustomers && (
        <div className="flex items-center justify-center py-8 text-muted-foreground text-sm">
          <Loader2 className="h-4 w-4 mr-2 animate-spin" />
          Searching...
        </div>
      )}

      {customerResults && customerResults.length === 0 && (
        <p className="text-center text-muted-foreground text-sm py-8">
          No customers found for &quot;{customerSearch}&quot;
        </p>
      )}

      {customerResults && customerResults.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {customerResults.map((c) => (
            <Card
              key={c.id}
              className={`cursor-pointer transition-all hover:shadow-md ${
                selectedCustomer?.id === c.id
                  ? "border-primary ring-2 ring-primary/20"
                  : ""
              }`}
              onClick={() => setSelectedCustomer(c)}
            >
              <CardContent className="p-4">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-sm font-sans font-semibold">
                      {c.firstName} {c.lastName}
                    </p>
                    {c.email && (
                      <p className="text-xs text-muted-foreground">{c.email}</p>
                    )}
                    {c.phoneNumber && (
                      <p className="text-xs text-muted-foreground">
                        {c.phoneNumber}
                      </p>
                    )}
                  </div>
                  <div className="text-right">
                    {c.nationalId && (
                      <p className="text-[10px] font-mono text-muted-foreground">
                        ID: {c.nationalId}
                      </p>
                    )}
                    {c.kycTier !== undefined && (
                      <Badge variant="outline" className="text-[9px] mt-1">
                        KYC Tier {c.kycTier}
                      </Badge>
                    )}
                  </div>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {selectedCustomer && (
        <Card className="border-primary bg-primary/5">
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="h-10 w-10 rounded-full bg-primary/10 flex items-center justify-center">
                <User className="h-5 w-5 text-primary" />
              </div>
              <div>
                <p className="text-sm font-sans font-semibold">
                  {selectedCustomer.firstName} {selectedCustomer.lastName}
                </p>
                <p className="text-xs text-muted-foreground">
                  {selectedCustomer.email ?? selectedCustomer.phoneNumber ?? ""}
                </p>
              </div>
              <Badge className="ml-auto" variant="outline">
                Selected
              </Badge>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );

  const renderStep1 = () => (
    <div className="space-y-4">
      {loadingProducts && (
        <div className="flex items-center justify-center py-8 text-muted-foreground text-sm">
          <Loader2 className="h-4 w-4 mr-2 animate-spin" />
          Loading products...
        </div>
      )}

      {!loadingProducts && products.length === 0 && (
        <p className="text-center text-muted-foreground text-sm py-8">
          No deposit products available.
        </p>
      )}

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
        {products
          .filter((p) => p.status === "ACTIVE")
          .map((product) => (
            <Card
              key={product.id}
              className={`cursor-pointer transition-all hover:shadow-md ${
                selectedProduct?.id === product.id
                  ? "border-primary ring-2 ring-primary/20"
                  : ""
              }`}
              onClick={() => setSelectedProduct(product)}
            >
              <CardHeader className="pb-2 p-4">
                <div className="flex items-start justify-between">
                  <div>
                    <CardTitle className="text-sm font-sans">
                      {product.name}
                    </CardTitle>
                    <CardDescription className="text-[10px] font-mono">
                      {product.productCode}
                    </CardDescription>
                  </div>
                  {selectedProduct?.id === product.id && (
                    <div className="h-5 w-5 rounded-full bg-primary flex items-center justify-center">
                      <Check className="h-3 w-3 text-primary-foreground" />
                    </div>
                  )}
                </div>
              </CardHeader>
              <CardContent className="p-4 pt-0">
                <Badge
                  variant="outline"
                  className={`text-[9px] font-sans mb-2 ${
                    categoryColors[product.productCategory] ?? ""
                  }`}
                >
                  {product.productCategory.replace("_", " ")}
                </Badge>
                <div className="grid grid-cols-2 gap-2 pt-2 border-t">
                  <div>
                    <p className="text-[9px] text-muted-foreground font-sans">
                      Interest Rate
                    </p>
                    <p className="text-xs font-mono font-semibold">
                      {product.interestRate}% p.a.
                    </p>
                  </div>
                  <div>
                    <p className="text-[9px] text-muted-foreground font-sans">
                      Min Balance
                    </p>
                    <p className="text-xs font-mono font-semibold">
                      {formatKES(product.minOpeningBalance)}
                    </p>
                  </div>
                </div>
                {product.description && (
                  <p className="text-[10px] text-muted-foreground mt-2 line-clamp-2">
                    {product.description}
                  </p>
                )}
              </CardContent>
            </Card>
          ))}
      </div>
    </div>
  );

  const renderStep2 = () => (
    <div className="max-w-lg mx-auto space-y-4">
      <div className="space-y-2">
        <Label htmlFor="accountName" className="text-xs font-sans">
          Account Name
        </Label>
        <Input
          id="accountName"
          placeholder="e.g., Personal Savings"
          value={accountName}
          onChange={(e) => setAccountName(e.target.value)}
        />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="currency" className="text-xs font-sans">
            Currency
          </Label>
          <Select value={currency} onValueChange={setCurrency}>
            <SelectTrigger id="currency">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="KES">KES</SelectItem>
              <SelectItem value="USD">USD</SelectItem>
              <SelectItem value="EUR">EUR</SelectItem>
              <SelectItem value="GBP">GBP</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="branch" className="text-xs font-sans">
            Branch
          </Label>
          <Input
            id="branch"
            placeholder="Branch code"
            value={branch}
            onChange={(e) => setBranch(e.target.value)}
          />
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="initialDeposit" className="text-xs font-sans">
          Initial Deposit Amount
        </Label>
        <Input
          id="initialDeposit"
          type="number"
          placeholder="0.00"
          value={initialDeposit}
          onChange={(e) => setInitialDeposit(e.target.value)}
        />
        {selectedProduct && parseFloat(initialDeposit || "0") > 0 &&
          parseFloat(initialDeposit) < selectedProduct.minOpeningBalance && (
            <p className="text-[10px] text-destructive">
              Minimum opening balance is {formatKES(selectedProduct.minOpeningBalance)}
            </p>
          )}
      </div>

      {selectedProduct?.productCategory === "FIXED_DEPOSIT" && (
        <>
          <div className="space-y-2">
            <Label htmlFor="termDays" className="text-xs font-sans">
              Term (Days)
            </Label>
            <Input
              id="termDays"
              type="number"
              placeholder={
                selectedProduct.minTermDays
                  ? `Min: ${selectedProduct.minTermDays} days`
                  : "e.g., 90"
              }
              value={termDays}
              onChange={(e) => setTermDays(e.target.value)}
            />
            {selectedProduct.minTermDays && selectedProduct.maxTermDays && (
              <p className="text-[10px] text-muted-foreground">
                Allowed: {selectedProduct.minTermDays} - {selectedProduct.maxTermDays} days
              </p>
            )}
          </div>

          <div className="flex items-center gap-3">
            <Label htmlFor="autoRenew" className="text-xs font-sans">
              Auto Renew
            </Label>
            <button
              id="autoRenew"
              type="button"
              role="switch"
              aria-checked={autoRenew}
              onClick={() => setAutoRenew(!autoRenew)}
              className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
                autoRenew ? "bg-primary" : "bg-muted"
              }`}
            >
              <span
                className={`inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform ${
                  autoRenew ? "translate-x-4" : "translate-x-0.5"
                }`}
              />
            </button>
          </div>
        </>
      )}

      <div className="space-y-2">
        <Label htmlFor="interestOverride" className="text-xs font-sans">
          Interest Rate Override (optional)
        </Label>
        <Input
          id="interestOverride"
          type="number"
          step="0.01"
          placeholder={
            selectedProduct
              ? `Default: ${selectedProduct.interestRate}%`
              : "e.g., 5.5"
          }
          value={interestRateOverride}
          onChange={(e) => setInterestRateOverride(e.target.value)}
        />
      </div>
    </div>
  );

  const renderStep3 = () => (
    <div className="max-w-lg mx-auto space-y-4">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-sans">Customer</CardTitle>
        </CardHeader>
        <CardContent className="space-y-1">
          <p className="text-sm font-sans font-semibold">
            {selectedCustomer?.firstName} {selectedCustomer?.lastName}
          </p>
          <p className="text-xs text-muted-foreground">
            {selectedCustomer?.email ?? selectedCustomer?.phoneNumber ?? "N/A"}
          </p>
          {selectedCustomer?.kycTier !== undefined && (
            <Badge variant="outline" className="text-[9px]">
              KYC Tier {selectedCustomer.kycTier}
            </Badge>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-sans">Product</CardTitle>
        </CardHeader>
        <CardContent className="space-y-1">
          <p className="text-sm font-sans font-semibold">
            {selectedProduct?.name}
          </p>
          <p className="text-[10px] font-mono text-muted-foreground">
            {selectedProduct?.productCode}
          </p>
          <div className="flex items-center gap-2 mt-1">
            <Badge
              variant="outline"
              className={`text-[9px] ${
                categoryColors[selectedProduct?.productCategory ?? ""] ?? ""
              }`}
            >
              {selectedProduct?.productCategory.replace("_", " ")}
            </Badge>
            <span className="text-xs font-mono">
              {selectedProduct?.interestRate}% p.a.
            </span>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-sans">Account Details</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-3 text-xs">
            <div>
              <p className="text-muted-foreground font-sans">Account Name</p>
              <p className="font-sans font-medium">{accountName}</p>
            </div>
            <div>
              <p className="text-muted-foreground font-sans">Currency</p>
              <p className="font-sans font-medium">{currency}</p>
            </div>
            {branch && (
              <div>
                <p className="text-muted-foreground font-sans">Branch</p>
                <p className="font-sans font-medium">{branch}</p>
              </div>
            )}
            {initialDeposit && (
              <div>
                <p className="text-muted-foreground font-sans">Initial Deposit</p>
                <p className="font-mono font-medium">
                  {formatKES(parseFloat(initialDeposit))}
                </p>
              </div>
            )}
            {selectedProduct?.productCategory === "FIXED_DEPOSIT" && termDays && (
              <div>
                <p className="text-muted-foreground font-sans">Term</p>
                <p className="font-sans font-medium">{termDays} days</p>
              </div>
            )}
            {selectedProduct?.productCategory === "FIXED_DEPOSIT" && (
              <div>
                <p className="text-muted-foreground font-sans">Auto Renew</p>
                <p className="font-sans font-medium">
                  {autoRenew ? "Yes" : "No"}
                </p>
              </div>
            )}
            {interestRateOverride && (
              <div>
                <p className="text-muted-foreground font-sans">Rate Override</p>
                <p className="font-mono font-medium">
                  {interestRateOverride}%
                </p>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );

  return (
    <DashboardLayout
      title="Open New Account"
      subtitle="Multi-step account opening wizard"
      breadcrumbs={[
        { label: "Home", href: "/" },
        { label: "Accounts", href: "/accounts" },
        { label: "Open Account" },
      ]}
    >
      <div className="space-y-6">
        {renderStepIndicator()}

        <Card>
          <CardHeader>
            <CardTitle className="text-base font-sans">
              {STEPS[step].label}
            </CardTitle>
            <CardDescription className="text-xs font-sans">
              {step === 0 && "Search and select the customer for the new account."}
              {step === 1 && "Choose a deposit product for this account."}
              {step === 2 && "Configure account details and initial deposit."}
              {step === 3 && "Review all details before submitting."}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {step === 0 && renderStep0()}
            {step === 1 && renderStep1()}
            {step === 2 && renderStep2()}
            {step === 3 && renderStep3()}
          </CardContent>
        </Card>

        <div className="flex items-center justify-between">
          <Button
            variant="outline"
            size="sm"
            onClick={handleBack}
            disabled={step === 0}
            className="text-xs font-sans"
          >
            <ChevronLeft className="h-3.5 w-3.5 mr-1" />
            Back
          </Button>

          {step < 3 ? (
            <Button
              size="sm"
              onClick={handleNext}
              disabled={!canProceed()}
              className="text-xs font-sans"
            >
              Next
              <ChevronRight className="h-3.5 w-3.5 ml-1" />
            </Button>
          ) : (
            <Button
              size="sm"
              onClick={handleSubmit}
              disabled={openAccountMutation.isPending}
              className="text-xs font-sans"
            >
              {openAccountMutation.isPending ? (
                <>
                  <Loader2 className="h-3.5 w-3.5 mr-1 animate-spin" />
                  Submitting...
                </>
              ) : (
                <>
                  <Check className="h-3.5 w-3.5 mr-1" />
                  Submit
                </>
              )}
            </Button>
          )}
        </div>
      </div>
    </DashboardLayout>
  );
};

export default AccountOpeningPage;
