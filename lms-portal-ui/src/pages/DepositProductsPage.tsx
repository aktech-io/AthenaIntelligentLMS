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
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  depositProductService,
  type DepositProduct,
  type CreateDepositProductRequest,
} from "@/services/depositProductService";
import { useState } from "react";
import { useToast } from "@/hooks/use-toast";
import {
  Search,
  Plus,
  Play,
  Pause,
  Loader2,
} from "lucide-react";
import { formatKES } from "@/lib/format";

const categoryColors: Record<string, string> = {
  SAVINGS: "bg-green-100 text-green-800 border-green-300",
  CURRENT: "bg-blue-100 text-blue-800 border-blue-300",
  FIXED_DEPOSIT: "bg-purple-100 text-purple-800 border-purple-300",
  CALL_DEPOSIT: "bg-orange-100 text-orange-800 border-orange-300",
  WALLET: "bg-gray-100 text-gray-800 border-gray-300",
};

const statusColors: Record<string, string> = {
  ACTIVE: "bg-green-100 text-green-800 border-green-300",
  DRAFT: "bg-gray-100 text-gray-600 border-gray-300",
  PAUSED: "bg-amber-100 text-amber-800 border-amber-300",
  ARCHIVED: "bg-red-100 text-red-800 border-red-300",
};

const EMPTY_FORM: CreateDepositProductRequest = {
  productCode: "",
  name: "",
  description: "",
  productCategory: "SAVINGS",
  currency: "KES",
  interestRate: 0,
  interestCalcMethod: "DAILY_BALANCE",
  interestPostingFreq: "MONTHLY",
  interestCompoundFreq: "MONTHLY",
  accrualFrequency: "DAILY",
  minOpeningBalance: 0,
  minOperatingBalance: 0,
  minBalanceForInterest: 0,
  minTermDays: undefined,
  maxTermDays: undefined,
  earlyWithdrawalPenaltyRate: undefined,
  autoRenew: false,
  dormancyDaysThreshold: 180,
  dormancyChargeAmount: undefined,
  monthlyMaintenanceFee: undefined,
  maxWithdrawalsPerMonth: undefined,
};

const DepositProductsPage = () => {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const [search, setSearch] = useState("");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [form, setForm] = useState<CreateDepositProductRequest>({ ...EMPTY_FORM });

  const { data: productsPage, isLoading } = useQuery({
    queryKey: ["deposit-products"],
    queryFn: () => depositProductService.listProducts(0, 100),
    staleTime: 60_000,
  });

  const products = productsPage?.content ?? [];

  const filtered = products.filter(
    (p) =>
      !search ||
      p.name.toLowerCase().includes(search.toLowerCase()) ||
      p.productCode.toLowerCase().includes(search.toLowerCase()) ||
      p.productCategory.toLowerCase().includes(search.toLowerCase())
  );

  const createMutation = useMutation({
    mutationFn: (req: CreateDepositProductRequest) =>
      depositProductService.createProduct(req),
    onSuccess: () => {
      toast({ title: "Product created", description: "Deposit product has been created successfully." });
      queryClient.invalidateQueries({ queryKey: ["deposit-products"] });
      setDialogOpen(false);
      setForm({ ...EMPTY_FORM });
    },
    onError: (err: Error) => {
      toast({ title: "Failed to create product", description: err.message, variant: "destructive" });
    },
  });

  const activateMutation = useMutation({
    mutationFn: (id: string) => depositProductService.activateProduct(id),
    onSuccess: () => {
      toast({ title: "Product activated" });
      queryClient.invalidateQueries({ queryKey: ["deposit-products"] });
    },
    onError: (err: Error) => {
      toast({ title: "Failed to activate", description: err.message, variant: "destructive" });
    },
  });

  const deactivateMutation = useMutation({
    mutationFn: (id: string) => depositProductService.deactivateProduct(id),
    onSuccess: () => {
      toast({ title: "Product deactivated" });
      queryClient.invalidateQueries({ queryKey: ["deposit-products"] });
    },
    onError: (err: Error) => {
      toast({ title: "Failed to deactivate", description: err.message, variant: "destructive" });
    },
  });

  const updateField = <K extends keyof CreateDepositProductRequest>(
    field: K,
    value: CreateDepositProductRequest[K]
  ) => {
    setForm((prev) => ({ ...prev, [field]: value }));
  };

  const handleCreate = () => {
    if (!form.productCode.trim() || !form.name.trim()) {
      toast({ title: "Validation error", description: "Product code and name are required.", variant: "destructive" });
      return;
    }
    createMutation.mutate(form);
  };

  const renderCreateDialog = () => (
    <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
      <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="font-sans">Create Deposit Product</DialogTitle>
          <DialogDescription className="text-xs font-sans">
            Define a new deposit product with interest rates, limits, and configuration.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label className="text-xs font-sans">Product Code *</Label>
              <Input
                placeholder="e.g., SAV-001"
                value={form.productCode}
                onChange={(e) => updateField("productCode", e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label className="text-xs font-sans">Product Name *</Label>
              <Input
                placeholder="e.g., Premium Savings"
                value={form.name}
                onChange={(e) => updateField("name", e.target.value)}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label className="text-xs font-sans">Description</Label>
            <Input
              placeholder="Brief product description"
              value={form.description ?? ""}
              onChange={(e) => updateField("description", e.target.value)}
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label className="text-xs font-sans">Category</Label>
              <Select
                value={form.productCategory}
                onValueChange={(v) => updateField("productCategory", v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="SAVINGS">Savings</SelectItem>
                  <SelectItem value="CURRENT">Current</SelectItem>
                  <SelectItem value="FIXED_DEPOSIT">Fixed Deposit</SelectItem>
                  <SelectItem value="CALL_DEPOSIT">Call Deposit</SelectItem>
                  <SelectItem value="WALLET">Wallet</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label className="text-xs font-sans">Currency</Label>
              <Select
                value={form.currency ?? "KES"}
                onValueChange={(v) => updateField("currency", v)}
              >
                <SelectTrigger>
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
          </div>

          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-2">
              <Label className="text-xs font-sans">Interest Rate (%)</Label>
              <Input
                type="number"
                step="0.01"
                value={form.interestRate ?? 0}
                onChange={(e) => updateField("interestRate", parseFloat(e.target.value) || 0)}
              />
            </div>
            <div className="space-y-2">
              <Label className="text-xs font-sans">Calc Method</Label>
              <Select
                value={form.interestCalcMethod ?? "DAILY_BALANCE"}
                onValueChange={(v) => updateField("interestCalcMethod", v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="DAILY_BALANCE">Daily Balance</SelectItem>
                  <SelectItem value="MINIMUM_BALANCE">Minimum Balance</SelectItem>
                  <SelectItem value="AVERAGE_BALANCE">Average Balance</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label className="text-xs font-sans">Accrual Freq</Label>
              <Select
                value={form.accrualFrequency ?? "DAILY"}
                onValueChange={(v) => updateField("accrualFrequency", v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="DAILY">Daily</SelectItem>
                  <SelectItem value="MONTHLY">Monthly</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label className="text-xs font-sans">Posting Frequency</Label>
              <Select
                value={form.interestPostingFreq ?? "MONTHLY"}
                onValueChange={(v) => updateField("interestPostingFreq", v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="MONTHLY">Monthly</SelectItem>
                  <SelectItem value="QUARTERLY">Quarterly</SelectItem>
                  <SelectItem value="SEMI_ANNUALLY">Semi-Annually</SelectItem>
                  <SelectItem value="ANNUALLY">Annually</SelectItem>
                  <SelectItem value="ON_MATURITY">On Maturity</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label className="text-xs font-sans">Compounding Frequency</Label>
              <Select
                value={form.interestCompoundFreq ?? "MONTHLY"}
                onValueChange={(v) => updateField("interestCompoundFreq", v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="DAILY">Daily</SelectItem>
                  <SelectItem value="MONTHLY">Monthly</SelectItem>
                  <SelectItem value="QUARTERLY">Quarterly</SelectItem>
                  <SelectItem value="ANNUALLY">Annually</SelectItem>
                  <SelectItem value="NONE">None</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-2">
              <Label className="text-xs font-sans">Min Opening Balance</Label>
              <Input
                type="number"
                value={form.minOpeningBalance ?? 0}
                onChange={(e) => updateField("minOpeningBalance", parseFloat(e.target.value) || 0)}
              />
            </div>
            <div className="space-y-2">
              <Label className="text-xs font-sans">Min Operating Balance</Label>
              <Input
                type="number"
                value={form.minOperatingBalance ?? 0}
                onChange={(e) => updateField("minOperatingBalance", parseFloat(e.target.value) || 0)}
              />
            </div>
            <div className="space-y-2">
              <Label className="text-xs font-sans">Min Balance for Interest</Label>
              <Input
                type="number"
                value={form.minBalanceForInterest ?? 0}
                onChange={(e) => updateField("minBalanceForInterest", parseFloat(e.target.value) || 0)}
              />
            </div>
          </div>

          {form.productCategory === "FIXED_DEPOSIT" && (
            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label className="text-xs font-sans">Min Term (Days)</Label>
                <Input
                  type="number"
                  value={form.minTermDays ?? ""}
                  onChange={(e) =>
                    updateField("minTermDays", e.target.value ? parseInt(e.target.value) : undefined)
                  }
                />
              </div>
              <div className="space-y-2">
                <Label className="text-xs font-sans">Max Term (Days)</Label>
                <Input
                  type="number"
                  value={form.maxTermDays ?? ""}
                  onChange={(e) =>
                    updateField("maxTermDays", e.target.value ? parseInt(e.target.value) : undefined)
                  }
                />
              </div>
              <div className="space-y-2">
                <Label className="text-xs font-sans">Early Withdrawal Penalty (%)</Label>
                <Input
                  type="number"
                  step="0.01"
                  value={form.earlyWithdrawalPenaltyRate ?? ""}
                  onChange={(e) =>
                    updateField(
                      "earlyWithdrawalPenaltyRate",
                      e.target.value ? parseFloat(e.target.value) : undefined
                    )
                  }
                />
              </div>
            </div>
          )}

          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-2">
              <Label className="text-xs font-sans">Dormancy Threshold (Days)</Label>
              <Input
                type="number"
                value={form.dormancyDaysThreshold ?? 180}
                onChange={(e) => updateField("dormancyDaysThreshold", parseInt(e.target.value) || 180)}
              />
            </div>
            <div className="space-y-2">
              <Label className="text-xs font-sans">Dormancy Charge</Label>
              <Input
                type="number"
                value={form.dormancyChargeAmount ?? ""}
                onChange={(e) =>
                  updateField("dormancyChargeAmount", e.target.value ? parseFloat(e.target.value) : undefined)
                }
              />
            </div>
            <div className="space-y-2">
              <Label className="text-xs font-sans">Monthly Maintenance Fee</Label>
              <Input
                type="number"
                value={form.monthlyMaintenanceFee ?? ""}
                onChange={(e) =>
                  updateField("monthlyMaintenanceFee", e.target.value ? parseFloat(e.target.value) : undefined)
                }
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label className="text-xs font-sans">Max Withdrawals/Month</Label>
              <Input
                type="number"
                value={form.maxWithdrawalsPerMonth ?? ""}
                onChange={(e) =>
                  updateField("maxWithdrawalsPerMonth", e.target.value ? parseInt(e.target.value) : undefined)
                }
              />
            </div>
            <div className="flex items-end gap-3 pb-1">
              <Label className="text-xs font-sans">Auto Renew</Label>
              <button
                type="button"
                role="switch"
                aria-checked={form.autoRenew ?? false}
                onClick={() => updateField("autoRenew", !(form.autoRenew ?? false))}
                className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
                  form.autoRenew ? "bg-primary" : "bg-muted"
                }`}
              >
                <span
                  className={`inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform ${
                    form.autoRenew ? "translate-x-4" : "translate-x-0.5"
                  }`}
                />
              </button>
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setDialogOpen(false)}
            className="text-xs font-sans"
          >
            Cancel
          </Button>
          <Button
            size="sm"
            onClick={handleCreate}
            disabled={createMutation.isPending}
            className="text-xs font-sans"
          >
            {createMutation.isPending ? (
              <>
                <Loader2 className="h-3.5 w-3.5 mr-1 animate-spin" />
                Creating...
              </>
            ) : (
              <>
                <Plus className="h-3.5 w-3.5 mr-1" />
                Create Product
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );

  return (
    <DashboardLayout
      title="Deposit Products"
      subtitle="Manage savings, current, and fixed deposit product definitions"
      breadcrumbs={[
        { label: "Home", href: "/" },
        { label: "Products" },
        { label: "Deposit Products" },
      ]}
    >
      <div className="space-y-4">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <div className="relative w-full sm:w-72">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              placeholder="Search products..."
              className="pl-8 h-9 text-xs font-sans"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <Button
            size="sm"
            className="text-xs font-sans bg-primary hover:bg-primary/90"
            onClick={() => {
              setForm({ ...EMPTY_FORM });
              setDialogOpen(true);
            }}
          >
            <Plus className="mr-1.5 h-3.5 w-3.5" /> Create Product
          </Button>
        </div>

        {isLoading && (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            {Array.from({ length: 8 }).map((_, i) => (
              <Card key={i}>
                <CardContent className="p-4 space-y-3">
                  <Skeleton className="h-6 w-3/4" />
                  <Skeleton className="h-4 w-1/2" />
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-8 w-full" />
                </CardContent>
              </Card>
            ))}
          </div>
        )}

        {!isLoading && filtered.length === 0 && (
          <div className="text-muted-foreground p-8 text-center text-sm">
            {search ? "No products match your search." : "No deposit products available. Create one to get started."}
          </div>
        )}

        {!isLoading && filtered.length > 0 && (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            {filtered.map((product) => (
              <Card
                key={product.id}
                className="hover:shadow-md hover:border-accent/30 transition-all"
              >
                <CardHeader className="pb-2 p-4">
                  <div className="flex items-start justify-between">
                    <div>
                      <CardTitle className="text-sm font-sans">
                        {product.name}
                      </CardTitle>
                      <CardDescription className="text-[11px] font-mono">
                        {product.productCode}
                      </CardDescription>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="p-4 pt-0">
                  <div className="flex items-center gap-2 mb-3">
                    <Badge
                      variant="outline"
                      className={`text-[11px] font-sans font-semibold ${
                        categoryColors[product.productCategory] ?? ""
                      }`}
                    >
                      {product.productCategory.replace(/_/g, " ")}
                    </Badge>
                    <Badge
                      variant="outline"
                      className={`text-[11px] font-sans font-semibold ${
                        statusColors[product.status] ?? ""
                      }`}
                    >
                      {product.status}
                    </Badge>
                  </div>

                  <div className="grid grid-cols-2 gap-2 pt-2 border-t mb-3">
                    <div>
                      <p className="text-[11px] text-muted-foreground font-sans">
                        Interest Rate
                      </p>
                      <p className="text-xs font-mono font-semibold">
                        {product.interestRate}% p.a.
                      </p>
                    </div>
                    <div>
                      <p className="text-[11px] text-muted-foreground font-sans">
                        Min Balance
                      </p>
                      <p className="text-xs font-mono font-semibold">
                        {formatKES(product.minOpeningBalance)}
                      </p>
                    </div>
                  </div>

                  {product.description && (
                    <p className="text-[11px] text-muted-foreground mb-3 line-clamp-2">
                      {product.description}
                    </p>
                  )}

                  <div className="flex items-center gap-1.5 pt-2 border-t">
                    {product.status !== "ACTIVE" ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-[11px] h-7 font-sans flex-1 text-green-700 hover:text-green-800 hover:bg-green-50"
                        disabled={activateMutation.isPending}
                        onClick={() => activateMutation.mutate(product.id)}
                      >
                        {activateMutation.isPending ? (
                          <Loader2 className="h-3 w-3 mr-1 animate-spin" />
                        ) : (
                          <Play className="h-3 w-3 mr-1" />
                        )}
                        Activate
                      </Button>
                    ) : (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-[11px] h-7 font-sans flex-1 text-amber-700 hover:text-amber-800 hover:bg-amber-50"
                        disabled={deactivateMutation.isPending}
                        onClick={() => deactivateMutation.mutate(product.id)}
                      >
                        {deactivateMutation.isPending ? (
                          <Loader2 className="h-3 w-3 mr-1 animate-spin" />
                        ) : (
                          <Pause className="h-3 w-3 mr-1" />
                        )}
                        Deactivate
                      </Button>
                    )}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>

      {renderCreateDialog()}
    </DashboardLayout>
  );
};

export default DepositProductsPage;
