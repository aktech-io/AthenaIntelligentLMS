import { useState, useMemo } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { Checkbox } from "@/components/ui/checkbox";
import { Slider } from "@/components/ui/slider";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Sheet, SheetContent, SheetHeader, SheetTitle,
} from "@/components/ui/sheet";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from "@/components/ui/dialog";
import {
  ChevronLeft, ChevronRight, Save, Rocket, CheckCircle2, Plus, Trash2, Loader2,
  FileText, DollarSign, CalendarDays, BookOpen, GitBranch, Bell,
} from "lucide-react";
import { motion, AnimatePresence } from "framer-motion";
import { formatKES } from "@/lib/format";
import { chartOfAccounts, defaultGLMappings, productTemplates, type GLMapping } from "@/data/productConfig";
import { toast } from "@/hooks/use-toast";
import {
  BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid,
} from "recharts";
import { Link } from "react-router-dom";
import { productService } from "@/services/productService";

const STEPS = [
  { id: 1, label: "Identity & Eligibility", icon: FileText },
  { id: 2, label: "Financial Terms", icon: DollarSign },
  { id: 3, label: "Schedule Engine", icon: CalendarDays },
  { id: 4, label: "GL & Accounting", icon: BookOpen },
  { id: 5, label: "Approval Workflow", icon: GitBranch },
  { id: 6, label: "Documents & Notifications", icon: Bell },
];

interface FeeRow { name: string; type: string; amount: string; timing: string }
interface DocRow { name: string; required: boolean; formats: string; maxSize: string; expiryTracked: boolean }
interface NotifRow { event: string; sms: boolean; email: boolean; push: boolean }

const defaultFees: FeeRow[] = [
  { name: "Origination Fee", type: "Percentage", amount: "3.0%", timing: "On Disbursement" },
  { name: "Late Payment Fee", type: "Percentage", amount: "5.0%", timing: "On Default" },
];

const defaultDocs: DocRow[] = [
  { name: "National ID", required: true, formats: "PDF, JPG", maxSize: "5MB", expiryTracked: true },
  { name: "Bank Statements (3 months)", required: true, formats: "PDF", maxSize: "10MB", expiryTracked: false },
  { name: "Payslip", required: true, formats: "PDF, JPG", maxSize: "5MB", expiryTracked: false },
  { name: "Proof of Address", required: false, formats: "PDF, JPG", maxSize: "5MB", expiryTracked: true },
  { name: "Employment Letter", required: false, formats: "PDF", maxSize: "5MB", expiryTracked: true },
];

const defaultNotifs: NotifRow[] = [
  { event: "Application Received", sms: true, email: true, push: true },
  { event: "Approved", sms: true, email: true, push: true },
  { event: "Declined", sms: true, email: true, push: false },
  { event: "Payment Due", sms: true, email: false, push: true },
  { event: "Payment Overdue", sms: true, email: true, push: true },
  { event: "Loan Closure", sms: true, email: true, push: false },
];

const workflowStages = [
  { id: "auto", label: "Auto-approve", threshold: "Score > 700 & Amount < KES 50K" },
  { id: "officer", label: "Loan Officer", threshold: "All applications" },
  { id: "analyst", label: "Credit Analyst", threshold: "> KES 500K" },
  { id: "branch", label: "Branch Manager", threshold: "> KES 2M" },
  { id: "committee", label: "Credit Committee", threshold: "> KES 5M" },
  { id: "board", label: "Board Approval", threshold: "> KES 20M" },
];

const ProductConfigPage = () => {
  const [step, setStep] = useState(1);
  const [templatesOpen, setTemplatesOpen] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [activating, setActivating] = useState(false);

  // Step 1
  const [productName, setProductName] = useState("");
  const [productCode, setProductCode] = useState("");
  const [productType, setProductType] = useState("Personal Loan");
  const [currency, setCurrency] = useState("KES");
  const [segments, setSegments] = useState<string[]>(["Retail"]);
  const [minAge, setMinAge] = useState("18");
  const [maxAge, setMaxAge] = useState("65");
  const [minKyc, setMinKyc] = useState("2");
  const [description, setDescription] = useState("");

  // Step 2
  const [minAmount, setMinAmount] = useState(10000);
  const [maxAmount, setMaxAmount] = useState(5000000);
  const [defaultAmount, setDefaultAmount] = useState(100000);
  const [minTenor, setMinTenor] = useState(3);
  const [maxTenor, setMaxTenor] = useState(60);
  const [defaultTenor, setDefaultTenor] = useState(24);
  const [interestMethod, setInterestMethod] = useState("Declining Balance (EMI)");
  const [rateType, setRateType] = useState("Fixed");
  const [annualRate, setAnnualRate] = useState(24.8);
  const [repaymentFreq, setRepaymentFreq] = useState("Monthly");
  const [gracePeriod, setGracePeriod] = useState("7");
  const [fees, setFees] = useState<FeeRow[]>(defaultFees);

  // Step 3
  const [paymentDay, setPaymentDay] = useState("1");
  const [firstPaymentOffset, setFirstPaymentOffset] = useState("30");
  const [roundingMethod, setRoundingMethod] = useState("Nearest KES 1");
  const [earlySettlement, setEarlySettlement] = useState("Fee Applies");
  const [earlySettlementFee, setEarlySettlementFee] = useState("2 months interest");
  const [overpaymentHandling, setOverpaymentHandling] = useState("Reduce Principal");

  // Step 4
  const [glMappings, setGlMappings] = useState<GLMapping[]>(defaultGLMappings);

  // Step 5
  const [enabledStages, setEnabledStages] = useState<string[]>(["officer", "analyst", "committee"]);
  const [requiresTwoPersonAuth, setRequiresTwoPersonAuth] = useState(false);
  const [authThreshold, setAuthThreshold] = useState("");

  // Step 6
  const [docs, setDocs] = useState<DocRow[]>(defaultDocs);
  const [notifs, setNotifs] = useState<NotifRow[]>(defaultNotifs);

  // EMI calc
  const emiCalc = useMemo(() => {
    const P = defaultAmount;
    const r = annualRate / 100 / 12;
    const n = defaultTenor;
    if (r === 0 || n === 0) return { emi: 0, totalInterest: 0, totalCost: 0, apr: 0, schedule: [] };
    const emi = interestMethod === "Flat Rate"
      ? (P + P * (annualRate / 100) * (n / 12)) / n
      : (P * r * Math.pow(1 + r, n)) / (Math.pow(1 + r, n) - 1);

    const totalCost = emi * n;
    const totalInterest = totalCost - P;

    const schedule = Array.from({ length: Math.min(n, 60) }, (_, i) => {
      const interestPortion = interestMethod === "Flat Rate"
        ? (P * (annualRate / 100) * (n / 12)) / n
        : (P * Math.pow(1 + r, i) - P * (Math.pow(1 + r, i) - 1) / ((Math.pow(1 + r, n) - 1) / (r * Math.pow(1 + r, n)))) * r;
      const principalPortion = emi - interestPortion;
      return { month: i + 1, principal: Math.max(0, principalPortion), interest: Math.max(0, interestPortion) };
    });

    return { emi, totalInterest, totalCost, apr: annualRate, schedule };
  }, [defaultAmount, defaultTenor, annualRate, interestMethod]);

  const loadTemplate = (tplId: string) => {
    const tpl = productTemplates.find(t => t.id === tplId);
    if (!tpl) return;
    // Pre-fill based on template
    if (tplId === "tpl-1") { setProductName("Digital Nano-Loan"); setProductCode("DNL-01"); setProductType("Nano-Loan"); setMinAmount(500); setMaxAmount(10000); setDefaultAmount(2000); setMinTenor(1); setMaxTenor(3); setDefaultTenor(1); setAnnualRate(42); setInterestMethod("Flat Rate"); setRepaymentFreq("Daily"); }
    if (tplId === "tpl-2") { setProductName("Consumer Personal Loan"); setProductCode("CPL-01"); setProductType("Personal Loan"); setMinAmount(10000); setMaxAmount(5000000); setDefaultAmount(100000); setMinTenor(3); setMaxTenor(60); setDefaultTenor(24); setAnnualRate(24.8); setInterestMethod("Declining Balance (EMI)"); setRepaymentFreq("Monthly"); }
    if (tplId === "tpl-3") { setProductName("BNPL 3-Month Interest-Free"); setProductCode("BNPL-01"); setProductType("BNPL"); setMinAmount(1000); setMaxAmount(200000); setDefaultAmount(15000); setMinTenor(3); setMaxTenor(3); setDefaultTenor(3); setAnnualRate(0); setInterestMethod("Flat Rate"); setRepaymentFreq("Monthly"); }
    if (tplId === "tpl-4") { setProductName("SME Business Term Loan"); setProductCode("SME-01"); setProductType("Business Loan"); setMinAmount(100000); setMaxAmount(20000000); setDefaultAmount(1000000); setMinTenor(6); setMaxTenor(60); setDefaultTenor(24); setAnnualRate(22.4); setInterestMethod("Declining Balance (EMI)"); setRepaymentFreq("Monthly"); }
    if (tplId === "tpl-5") { setProductName("Group Solidarity Loan"); setProductCode("GSL-01"); setProductType("Group Loan"); setMinAmount(5000); setMaxAmount(500000); setDefaultAmount(50000); setMinTenor(3); setMaxTenor(24); setDefaultTenor(12); setAnnualRate(18); setInterestMethod("Flat Rate"); setRepaymentFreq("Weekly"); }
    setTemplatesOpen(false);
    toast({ title: "Template loaded", description: `${tpl.name} configuration pre-filled across all steps.` });
  };

  const handleActivate = async () => {
    setActivating(true);
    try {
      // Map UI state to backend DTO
      const typeMap: Record<string, string> = {
        "Personal Loan": "PERSONAL_LOAN",
        "Nano-Loan": "NANO_LOAN",
        "BNPL": "BNPL",
        "Business Loan": "SME_LOAN",
        "SME Loan": "SME_LOAN",
        "Group Loan": "PERSONAL_LOAN",
        "Emergency Loan": "NANO_LOAN",
        "Staff Loan": "PERSONAL_LOAN",
        "Agricultural Loan": "PERSONAL_LOAN",
        "Asset Finance": "SME_LOAN",
      };
      const scheduleMap: Record<string, string> = {
        "Declining Balance (EMI)": "EMI",
        "Flat Rate": "FLAT_RATE",
      };
      const freqMap: Record<string, string> = {
        "Daily": "DAILY",
        "Weekly": "WEEKLY",
        "Bi-Weekly": "BIWEEKLY",
        "Monthly": "MONTHLY",
        "Quarterly": "QUARTERLY",
      };
      await productService.createProduct({
        name: productName,
        productCode: productCode,
        productType: typeMap[productType] ?? "PERSONAL_LOAN",
        currency: "KES",  // org currency — single currency enforced
        nominalRate: annualRate,
        minAmount: minAmount,
        maxAmount: maxAmount,
        minTenorDays: minTenor * 30,
        maxTenorDays: maxTenor * 30,
        gracePeriodDays: parseInt(gracePeriod) || 7,
        repaymentFrequency: freqMap[repaymentFreq] ?? "MONTHLY",
        scheduleType: scheduleMap[interestMethod] ?? "EMI",
        requiresTwoPersonAuth: requiresTwoPersonAuth,
        authThresholdAmount: requiresTwoPersonAuth
          ? parseFloat(authThreshold) || 0
          : undefined,
      });
      setConfirmOpen(false);
      toast({ title: "Product Created", description: `${productName} has been created and is now pending activation.` });
    } catch (err: unknown) {
      toast({
        title: "Failed to create product",
        description: err instanceof Error ? err.message : "Unknown error",
        variant: "destructive",
      });
    } finally {
      setActivating(false);
    }
  };

  const addFee = () => setFees([...fees, { name: "", type: "Percentage", amount: "", timing: "On Disbursement" }]);
  const removeFee = (i: number) => setFees(fees.filter((_, idx) => idx !== i));
  const addDoc = () => setDocs([...docs, { name: "", required: false, formats: "PDF", maxSize: "5MB", expiryTracked: false }]);
  const removeDoc = (i: number) => setDocs(docs.filter((_, idx) => idx !== i));

  const toggleSegment = (seg: string) =>
    setSegments(segments.includes(seg) ? segments.filter(s => s !== seg) : [...segments, seg]);

  const toggleStage = (stageId: string) =>
    setEnabledStages(enabledStages.includes(stageId) ? enabledStages.filter(s => s !== stageId) : [...enabledStages, stageId]);

  const setupComplete =
    typeof window !== "undefined" && localStorage.getItem("athena_setup_complete") === "true";

  return (
    <DashboardLayout
      title="Product Configuration Engine"
      subtitle="Build and configure lending products with no code"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Products", href: "/products" }, { label: "Product Config Engine" }]}
    >
      <div className="space-y-4">
        {/* Setup guard banner */}
        {!setupComplete && (
          <div className="flex items-center gap-3 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
            <span>
              Complete the Setup Wizard first before configuring products.
            </span>
            <Link to="/setup-wizard" className="ml-auto underline font-medium hover:text-amber-900 shrink-0">
              Go to Setup Wizard
            </Link>
          </div>
        )}

        {/* Step indicator */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1">
            {STEPS.map((s, i) => {
              const Icon = s.icon;
              const isActive = s.id === step;
              const isDone = s.id < step;
              return (
                <button
                  key={s.id}
                  onClick={() => setStep(s.id)}
                  className={`flex items-center gap-1.5 px-3 py-1.5 rounded-full text-[11px] font-sans transition-colors ${
                    isActive ? "bg-primary text-primary-foreground" : isDone ? "bg-success/15 text-success" : "text-muted-foreground hover:bg-muted"
                  }`}
                >
                  {isDone ? <CheckCircle2 className="h-3.5 w-3.5" /> : <Icon className="h-3.5 w-3.5" />}
                  <span className="hidden lg:inline">{s.label}</span>
                  <span className="lg:hidden">{s.id}</span>
                </button>
              );
            })}
          </div>
          <Button variant="outline" size="sm" className="text-xs font-sans" onClick={() => setTemplatesOpen(true)}>
            Load Template
          </Button>
        </div>

        <AnimatePresence mode="wait">
          <motion.div
            key={step}
            initial={{ opacity: 0, x: 20 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: -20 }}
            transition={{ duration: 0.2 }}
          >
            {/* STEP 1 */}
            {step === 1 && (
              <Card>
                <CardHeader><CardTitle className="text-sm font-sans font-semibold">Step 1: Identity & Eligibility</CardTitle></CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div>
                      <Label className="text-[10px] uppercase tracking-wider font-sans">Product Name</Label>
                      <Input value={productName} onChange={e => setProductName(e.target.value)} placeholder="e.g. Consumer Personal Loan" className="mt-1 text-xs font-sans" />
                    </div>
                    <div>
                      <Label className="text-[10px] uppercase tracking-wider font-sans">Product Code</Label>
                      <Input value={productCode} onChange={e => setProductCode(e.target.value)} placeholder="Auto-generated" className="mt-1 text-xs font-mono" />
                    </div>
                    <div>
                      <Label className="text-[10px] uppercase tracking-wider font-sans">Product Type</Label>
                      <Select value={productType} onValueChange={setProductType}>
                        <SelectTrigger className="mt-1 text-xs font-sans"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          {["Personal Loan", "Business Loan", "BNPL", "Nano-Loan", "Savings", "Wallet", "Float", "Group Loan"].map(t => (
                            <SelectItem key={t} value={t}>{t}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div>
                      <Label className="text-[10px] uppercase tracking-wider font-sans">Currency</Label>
                      <Select value={currency} onValueChange={setCurrency}>
                        <SelectTrigger className="mt-1 text-xs font-sans"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          {["KES", "USD", "UGX", "TZS", "RWF"].map(c => (
                            <SelectItem key={c} value={c}>{c}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>

                  <div>
                    <Label className="text-[10px] uppercase tracking-wider font-sans">Target Segments</Label>
                    <div className="flex gap-2 mt-1.5">
                      {["Retail", "SME", "Corporate", "MFI", "Agri"].map(seg => (
                        <button
                          key={seg}
                          onClick={() => toggleSegment(seg)}
                          className={`px-3 py-1.5 rounded-full text-[11px] font-sans border transition-colors ${
                            segments.includes(seg) ? "bg-primary text-primary-foreground border-primary" : "border-border text-muted-foreground hover:bg-muted"
                          }`}
                        >
                          {seg}
                        </button>
                      ))}
                    </div>
                  </div>

                  <div className="grid grid-cols-3 gap-4">
                    <div>
                      <Label className="text-[10px] uppercase tracking-wider font-sans">Min Customer Age</Label>
                      <Input value={minAge} onChange={e => setMinAge(e.target.value)} className="mt-1 text-xs font-mono" />
                    </div>
                    <div>
                      <Label className="text-[10px] uppercase tracking-wider font-sans">Max Customer Age</Label>
                      <Input value={maxAge} onChange={e => setMaxAge(e.target.value)} className="mt-1 text-xs font-mono" />
                    </div>
                    <div>
                      <Label className="text-[10px] uppercase tracking-wider font-sans">Min KYC Tier</Label>
                      <Select value={minKyc} onValueChange={setMinKyc}>
                        <SelectTrigger className="mt-1 text-xs font-sans"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          {["0", "1", "2", "3"].map(t => (
                            <SelectItem key={t} value={t}>Tier {t}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>

                  <div>
                    <Label className="text-[10px] uppercase tracking-wider font-sans">Product Description</Label>
                    <Textarea value={description} onChange={e => setDescription(e.target.value)} placeholder="Describe the product..." className="mt-1 text-xs font-sans" rows={3} />
                  </div>
                </CardContent>
              </Card>
            )}

            {/* STEP 2 */}
            {step === 2 && (
              <div className="grid grid-cols-1 lg:grid-cols-5 gap-4">
                <div className="lg:col-span-3 space-y-4">
                  <Card>
                    <CardHeader><CardTitle className="text-sm font-sans font-semibold">Principal</CardTitle></CardHeader>
                    <CardContent className="space-y-4">
                      <div className="grid grid-cols-3 gap-3">
                        <div>
                          <Label className="text-[10px] uppercase tracking-wider font-sans">Minimum ({currency})</Label>
                          <Input value={minAmount} onChange={e => setMinAmount(+e.target.value)} type="number" className="mt-1 text-xs font-mono" />
                        </div>
                        <div>
                          <Label className="text-[10px] uppercase tracking-wider font-sans">Maximum ({currency})</Label>
                          <Input value={maxAmount} onChange={e => setMaxAmount(+e.target.value)} type="number" className="mt-1 text-xs font-mono" />
                        </div>
                        <div>
                          <Label className="text-[10px] uppercase tracking-wider font-sans">Default ({currency})</Label>
                          <Input value={defaultAmount} onChange={e => setDefaultAmount(+e.target.value)} type="number" className="mt-1 text-xs font-mono" />
                        </div>
                      </div>
                      <Slider value={[defaultAmount]} min={minAmount} max={maxAmount} step={1000} onValueChange={([v]) => setDefaultAmount(v)} />
                    </CardContent>
                  </Card>

                  <Card>
                    <CardHeader><CardTitle className="text-sm font-sans font-semibold">Tenor (Months)</CardTitle></CardHeader>
                    <CardContent className="space-y-4">
                      <div className="grid grid-cols-3 gap-3">
                        <div>
                          <Label className="text-[10px] uppercase tracking-wider font-sans">Minimum</Label>
                          <Input value={minTenor} onChange={e => setMinTenor(+e.target.value)} type="number" className="mt-1 text-xs font-mono" />
                        </div>
                        <div>
                          <Label className="text-[10px] uppercase tracking-wider font-sans">Maximum</Label>
                          <Input value={maxTenor} onChange={e => setMaxTenor(+e.target.value)} type="number" className="mt-1 text-xs font-mono" />
                        </div>
                        <div>
                          <Label className="text-[10px] uppercase tracking-wider font-sans">Default</Label>
                          <Input value={defaultTenor} onChange={e => setDefaultTenor(+e.target.value)} type="number" className="mt-1 text-xs font-mono" />
                        </div>
                      </div>
                      <Slider value={[defaultTenor]} min={minTenor} max={maxTenor} step={1} onValueChange={([v]) => setDefaultTenor(v)} />
                    </CardContent>
                  </Card>

                  <Card>
                    <CardHeader><CardTitle className="text-sm font-sans font-semibold">Interest</CardTitle></CardHeader>
                    <CardContent className="space-y-3">
                      <div className="grid grid-cols-3 gap-3">
                        <div>
                          <Label className="text-[10px] uppercase tracking-wider font-sans">Method</Label>
                          <Select value={interestMethod} onValueChange={setInterestMethod}>
                            <SelectTrigger className="mt-1 text-xs font-sans"><SelectValue /></SelectTrigger>
                            <SelectContent>
                              {["Flat Rate", "Declining Balance (EMI)", "Actuarial", "Daily Simple Interest"].map(m => (
                                <SelectItem key={m} value={m}>{m}</SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                        <div>
                          <Label className="text-[10px] uppercase tracking-wider font-sans">Rate Type</Label>
                          <Select value={rateType} onValueChange={setRateType}>
                            <SelectTrigger className="mt-1 text-xs font-sans"><SelectValue /></SelectTrigger>
                            <SelectContent>
                              {["Fixed", "Variable", "Tiered by Risk Band"].map(r => (
                                <SelectItem key={r} value={r}>{r}</SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                        <div>
                          <Label className="text-[10px] uppercase tracking-wider font-sans">Annual Rate (%)</Label>
                          <Input value={annualRate} onChange={e => setAnnualRate(+e.target.value)} type="number" step="0.1" className="mt-1 text-xs font-mono" />
                        </div>
                      </div>
                    </CardContent>
                  </Card>

                  <Card>
                    <CardHeader>
                      <div className="flex items-center justify-between">
                        <CardTitle className="text-sm font-sans font-semibold">Fees</CardTitle>
                        <Button variant="outline" size="sm" className="text-[10px] h-7 font-sans" onClick={addFee}>
                          <Plus className="h-3 w-3 mr-1" /> Add Fee
                        </Button>
                      </div>
                    </CardHeader>
                    <CardContent>
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead className="text-[10px] uppercase font-sans">Fee Name</TableHead>
                            <TableHead className="text-[10px] uppercase font-sans">Type</TableHead>
                            <TableHead className="text-[10px] uppercase font-sans">Amount</TableHead>
                            <TableHead className="text-[10px] uppercase font-sans">Timing</TableHead>
                            <TableHead className="text-[10px] uppercase font-sans w-10"></TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {fees.map((fee, i) => (
                            <TableRow key={i}>
                              <TableCell><Input value={fee.name} onChange={e => { const n = [...fees]; n[i].name = e.target.value; setFees(n); }} className="text-xs font-sans h-7" /></TableCell>
                              <TableCell>
                                <Select value={fee.type} onValueChange={v => { const n = [...fees]; n[i].type = v; setFees(n); }}>
                                  <SelectTrigger className="text-xs font-sans h-7"><SelectValue /></SelectTrigger>
                                  <SelectContent>
                                    {["Fixed", "Percentage", "Tiered"].map(t => <SelectItem key={t} value={t}>{t}</SelectItem>)}
                                  </SelectContent>
                                </Select>
                              </TableCell>
                              <TableCell><Input value={fee.amount} onChange={e => { const n = [...fees]; n[i].amount = e.target.value; setFees(n); }} className="text-xs font-mono h-7" /></TableCell>
                              <TableCell>
                                <Select value={fee.timing} onValueChange={v => { const n = [...fees]; n[i].timing = v; setFees(n); }}>
                                  <SelectTrigger className="text-xs font-sans h-7"><SelectValue /></SelectTrigger>
                                  <SelectContent>
                                    {["On Disbursement", "Upfront", "Monthly", "On Default", "Daily"].map(t => <SelectItem key={t} value={t}>{t}</SelectItem>)}
                                  </SelectContent>
                                </Select>
                              </TableCell>
                              <TableCell>
                                <Button variant="ghost" size="sm" className="h-7 w-7 p-0 text-destructive" onClick={() => removeFee(i)}>
                                  <Trash2 className="h-3 w-3" />
                                </Button>
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </CardContent>
                  </Card>

                  <Card>
                    <CardContent className="p-4">
                      <div className="grid grid-cols-2 gap-3">
                        <div>
                          <Label className="text-[10px] uppercase tracking-wider font-sans">Grace Period (days)</Label>
                          <Input value={gracePeriod} onChange={e => setGracePeriod(e.target.value)} className="mt-1 text-xs font-mono" />
                        </div>
                        <div>
                          <Label className="text-[10px] uppercase tracking-wider font-sans">Repayment Frequency</Label>
                          <Select value={repaymentFreq} onValueChange={setRepaymentFreq}>
                            <SelectTrigger className="mt-1 text-xs font-sans"><SelectValue /></SelectTrigger>
                            <SelectContent>
                              {["Daily", "Weekly", "Bi-weekly", "Monthly", "Quarterly", "Bullet"].map(f => (
                                <SelectItem key={f} value={f}>{f}</SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                </div>

                {/* Live EMI Preview Panel */}
                <div className="lg:col-span-2">
                  <Card className="sticky top-4 border-accent/30">
                    <CardHeader className="pb-2">
                      <CardTitle className="text-sm font-sans font-semibold">Live Product Preview</CardTitle>
                    </CardHeader>
                    <CardContent className="space-y-3">
                      <div className="grid grid-cols-2 gap-3">
                        <div className="p-3 bg-muted/50 rounded-lg text-center">
                          <p className="text-[9px] text-muted-foreground font-sans uppercase">Monthly EMI</p>
                          <p className="text-lg font-mono font-bold">{formatKES(Math.round(emiCalc.emi))}</p>
                        </div>
                        <div className="p-3 bg-muted/50 rounded-lg text-center">
                          <p className="text-[9px] text-muted-foreground font-sans uppercase">Total Interest</p>
                          <p className="text-lg font-mono font-bold">{formatKES(Math.round(emiCalc.totalInterest))}</p>
                        </div>
                        <div className="p-3 bg-muted/50 rounded-lg text-center">
                          <p className="text-[9px] text-muted-foreground font-sans uppercase">Total Cost</p>
                          <p className="text-lg font-mono font-bold">{formatKES(Math.round(emiCalc.totalCost))}</p>
                        </div>
                        <div className="p-3 bg-muted/50 rounded-lg text-center">
                          <p className="text-[9px] text-muted-foreground font-sans uppercase">APR / EIR</p>
                          <p className="text-lg font-mono font-bold">{emiCalc.apr.toFixed(1)}%</p>
                        </div>
                      </div>

                      <div className="pt-2">
                        <p className="text-[10px] text-muted-foreground font-sans uppercase tracking-wider mb-2">Amortisation — Principal vs Interest</p>
                        <ResponsiveContainer width="100%" height={200}>
                          <BarChart data={emiCalc.schedule.slice(0, 24)}>
                            <CartesianGrid strokeDasharray="3 3" stroke="hsl(214, 20%, 88%)" />
                            <XAxis dataKey="month" tick={{ fontSize: 9, fontFamily: "JetBrains Mono" }} />
                            <YAxis tick={{ fontSize: 9, fontFamily: "JetBrains Mono" }} tickFormatter={v => `${(v / 1000).toFixed(0)}K`} />
                            <Tooltip contentStyle={{ fontSize: 11, fontFamily: "DM Sans", borderRadius: 8 }} />
                            <Bar dataKey="principal" stackId="a" fill="hsl(214, 62%, 15%)" name="Principal" radius={[0, 0, 0, 0]} />
                            <Bar dataKey="interest" stackId="a" fill="hsl(42, 56%, 54%)" name="Interest" radius={[2, 2, 0, 0]} />
                          </BarChart>
                        </ResponsiveContainer>
                      </div>

                      <div className="pt-2 border-t space-y-1.5">
                        <p className="text-[10px] text-muted-foreground font-sans uppercase tracking-wider">Configuration Summary</p>
                        {[
                          ["Product", productName || "—"],
                          ["Amount Range", `${formatKES(minAmount)} — ${formatKES(maxAmount)}`],
                          ["Tenor", `${minTenor} — ${maxTenor} months`],
                          ["Rate", `${annualRate}% p.a. (${rateType})`],
                          ["Method", interestMethod],
                          ["Frequency", repaymentFreq],
                        ].map(([k, v]) => (
                          <div key={k} className="flex justify-between text-[10px]">
                            <span className="text-muted-foreground font-sans">{k}</span>
                            <span className="font-mono font-medium">{v}</span>
                          </div>
                        ))}
                      </div>
                    </CardContent>
                  </Card>
                </div>
              </div>
            )}

            {/* STEP 3 */}
            {step === 3 && (
              <Card>
                <CardHeader><CardTitle className="text-sm font-sans font-semibold">Step 3: Schedule Engine</CardTitle></CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div>
                      <Label className="text-[10px] uppercase tracking-wider font-sans">Payment Day of Month</Label>
                      <Select value={paymentDay} onValueChange={setPaymentDay}>
                        <SelectTrigger className="mt-1 text-xs font-sans"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          {["1", "15", "Last Day", "Custom"].map(d => <SelectItem key={d} value={d}>{d === "1" ? "1st" : d === "15" ? "15th" : d}</SelectItem>)}
                        </SelectContent>
                      </Select>
                    </div>
                    <div>
                      <Label className="text-[10px] uppercase tracking-wider font-sans">First Payment Offset (days after disbursement)</Label>
                      <Input value={firstPaymentOffset} onChange={e => setFirstPaymentOffset(e.target.value)} className="mt-1 text-xs font-mono" />
                    </div>
                    <div>
                      <Label className="text-[10px] uppercase tracking-wider font-sans">Rounding Method</Label>
                      <Select value={roundingMethod} onValueChange={setRoundingMethod}>
                        <SelectTrigger className="mt-1 text-xs font-sans"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          {["Nearest KES 0.01", "Nearest KES 1", "Nearest KES 10"].map(r => <SelectItem key={r} value={r}>{r}</SelectItem>)}
                        </SelectContent>
                      </Select>
                    </div>
                    <div>
                      <Label className="text-[10px] uppercase tracking-wider font-sans">Early Settlement</Label>
                      <Select value={earlySettlement} onValueChange={setEarlySettlement}>
                        <SelectTrigger className="mt-1 text-xs font-sans"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          {["Allowed", "Not Allowed", "Fee Applies"].map(e => <SelectItem key={e} value={e}>{e}</SelectItem>)}
                        </SelectContent>
                      </Select>
                    </div>
                    {earlySettlement === "Fee Applies" && (
                      <div>
                        <Label className="text-[10px] uppercase tracking-wider font-sans">Early Settlement Fee Formula</Label>
                        <Input value={earlySettlementFee} onChange={e => setEarlySettlementFee(e.target.value)} className="mt-1 text-xs font-sans" placeholder="e.g. 2 months interest" />
                      </div>
                    )}
                    <div>
                      <Label className="text-[10px] uppercase tracking-wider font-sans">Overpayment Handling</Label>
                      <Select value={overpaymentHandling} onValueChange={setOverpaymentHandling}>
                        <SelectTrigger className="mt-1 text-xs font-sans"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          {["Apply to Next Instalment", "Reduce Principal", "Return to Customer"].map(o => <SelectItem key={o} value={o}>{o}</SelectItem>)}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                </CardContent>
              </Card>
            )}

            {/* STEP 4 */}
            {step === 4 && (
              <Card>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-sm font-sans font-semibold">Step 4: GL & Accounting Rules</CardTitle>
                    <Button variant="outline" size="sm" className="text-[10px] h-7 font-sans" onClick={() => setGlMappings(defaultGLMappings)}>
                      Load Standard Template
                    </Button>
                  </div>
                </CardHeader>
                <CardContent>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="text-[10px] uppercase font-sans">Event</TableHead>
                        <TableHead className="text-[10px] uppercase font-sans">Debit Account</TableHead>
                        <TableHead className="text-[10px] uppercase font-sans">Credit Account</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {glMappings.map((m, i) => (
                        <TableRow key={i}>
                          <TableCell className="text-xs font-sans font-medium">{m.event}</TableCell>
                          <TableCell>
                            <Select value={m.debitAccount} onValueChange={v => { const n = [...glMappings]; n[i].debitAccount = v; setGlMappings(n); }}>
                              <SelectTrigger className="text-xs font-mono h-7"><SelectValue /></SelectTrigger>
                              <SelectContent>
                                {chartOfAccounts.map(a => <SelectItem key={a.code} value={a.code}>{a.code} — {a.name}</SelectItem>)}
                              </SelectContent>
                            </Select>
                          </TableCell>
                          <TableCell>
                            <Select value={m.creditAccount} onValueChange={v => { const n = [...glMappings]; n[i].creditAccount = v; setGlMappings(n); }}>
                              <SelectTrigger className="text-xs font-mono h-7"><SelectValue /></SelectTrigger>
                              <SelectContent>
                                {chartOfAccounts.map(a => <SelectItem key={a.code} value={a.code}>{a.code} — {a.name}</SelectItem>)}
                              </SelectContent>
                            </Select>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </CardContent>
              </Card>
            )}

            {/* STEP 5 */}
            {step === 5 && (
              <Card>
                <CardHeader><CardTitle className="text-sm font-sans font-semibold">Step 5: Approval Workflow Builder</CardTitle></CardHeader>
                <CardContent className="space-y-4">
                  <p className="text-xs text-muted-foreground font-sans">Toggle stages to build the approval pipeline. Each stage applies to loans above its threshold.</p>
                  <div className="space-y-3">
                    {workflowStages.map((stage) => {
                      const enabled = enabledStages.includes(stage.id);
                      return (
                        <div key={stage.id} className={`flex items-center justify-between p-3 rounded-lg border transition-colors ${enabled ? "border-primary/30 bg-primary/5" : "border-border"}`}>
                          <div className="flex items-center gap-3">
                            <Switch checked={enabled} onCheckedChange={() => toggleStage(stage.id)} />
                            <div>
                              <p className="text-xs font-sans font-medium">{stage.label}</p>
                              <p className="text-[10px] text-muted-foreground font-sans">{stage.threshold}</p>
                            </div>
                          </div>
                          {enabled && (
                            <Badge variant="outline" className="text-[9px] font-sans bg-primary/10 text-primary border-primary/20">
                              Step {enabledStages.indexOf(stage.id) + 1}
                            </Badge>
                          )}
                        </div>
                      );
                    })}
                  </div>

                  {/* Visual flow */}
                  <div className="pt-4 border-t">
                    <p className="text-[10px] text-muted-foreground font-sans uppercase tracking-wider mb-3">Approval Flow</p>
                    <div className="flex items-center gap-2 flex-wrap">
                      <Badge className="bg-muted text-muted-foreground font-sans text-[10px]">Application</Badge>
                      {enabledStages.map((id, i) => {
                        const stage = workflowStages.find(s => s.id === id);
                        return (
                          <span key={id} className="flex items-center gap-2">
                            <span className="text-muted-foreground">→</span>
                            <Badge variant="outline" className="text-[10px] font-sans bg-primary/10 text-primary border-primary/20">
                              {stage?.label}
                            </Badge>
                          </span>
                        );
                      })}
                      <span className="text-muted-foreground">→</span>
                      <Badge className="bg-success/15 text-success font-sans text-[10px]">Disbursement</Badge>
                    </div>
                  </div>

                  {/* Per-product dual approval override */}
                  <div className="pt-4 border-t space-y-3">
                    <div className={`flex items-center justify-between p-3 rounded-lg border transition-colors ${requiresTwoPersonAuth ? "border-primary/30 bg-primary/5" : "border-border"}`}>
                      <div className="flex items-center gap-3">
                        <Switch checked={requiresTwoPersonAuth} onCheckedChange={setRequiresTwoPersonAuth} />
                        <div>
                          <p className="text-xs font-sans font-medium">Require dual approval (maker-checker)</p>
                          <p className="text-[10px] text-muted-foreground font-sans">A second authoriser must approve before this product can be approved/disbursed.</p>
                        </div>
                      </div>
                    </div>
                    {requiresTwoPersonAuth && (
                      <div>
                        <Label className="text-[10px] uppercase tracking-wider font-sans">Approval threshold (KES)</Label>
                        <Input
                          value={authThreshold}
                          onChange={e => setAuthThreshold(e.target.value)}
                          type="number"
                          min="0"
                          placeholder="0"
                          className="mt-1 text-xs font-mono"
                        />
                        <p className="text-[10px] text-muted-foreground font-sans mt-1">0 or blank means dual approval is always required.</p>
                      </div>
                    )}
                  </div>
                </CardContent>
              </Card>
            )}

            {/* STEP 6 */}
            {step === 6 && (
              <div className="space-y-4">
                <Card>
                  <CardHeader>
                    <div className="flex items-center justify-between">
                      <CardTitle className="text-sm font-sans font-semibold">Document Checklist</CardTitle>
                      <Button variant="outline" size="sm" className="text-[10px] h-7 font-sans" onClick={addDoc}>
                        <Plus className="h-3 w-3 mr-1" /> Add Document
                      </Button>
                    </div>
                  </CardHeader>
                  <CardContent>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead className="text-[10px] uppercase font-sans">Document</TableHead>
                          <TableHead className="text-[10px] uppercase font-sans">Required</TableHead>
                          <TableHead className="text-[10px] uppercase font-sans">Formats</TableHead>
                          <TableHead className="text-[10px] uppercase font-sans">Max Size</TableHead>
                          <TableHead className="text-[10px] uppercase font-sans">Expiry Tracked</TableHead>
                          <TableHead className="text-[10px] uppercase font-sans w-10"></TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {docs.map((doc, i) => (
                          <TableRow key={i}>
                            <TableCell><Input value={doc.name} onChange={e => { const n = [...docs]; n[i].name = e.target.value; setDocs(n); }} className="text-xs font-sans h-7" /></TableCell>
                            <TableCell><Switch checked={doc.required} onCheckedChange={v => { const n = [...docs]; n[i].required = v; setDocs(n); }} /></TableCell>
                            <TableCell><Input value={doc.formats} onChange={e => { const n = [...docs]; n[i].formats = e.target.value; setDocs(n); }} className="text-xs font-sans h-7" /></TableCell>
                            <TableCell><Input value={doc.maxSize} onChange={e => { const n = [...docs]; n[i].maxSize = e.target.value; setDocs(n); }} className="text-xs font-sans h-7 w-16" /></TableCell>
                            <TableCell><Switch checked={doc.expiryTracked} onCheckedChange={v => { const n = [...docs]; n[i].expiryTracked = v; setDocs(n); }} /></TableCell>
                            <TableCell>
                              <Button variant="ghost" size="sm" className="h-7 w-7 p-0 text-destructive" onClick={() => removeDoc(i)}>
                                <Trash2 className="h-3 w-3" />
                              </Button>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle className="text-sm font-sans font-semibold">Notification Templates</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead className="text-[10px] uppercase font-sans">Event</TableHead>
                          <TableHead className="text-[10px] uppercase font-sans text-center">SMS</TableHead>
                          <TableHead className="text-[10px] uppercase font-sans text-center">Email</TableHead>
                          <TableHead className="text-[10px] uppercase font-sans text-center">Push</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {notifs.map((n, i) => (
                          <TableRow key={i}>
                            <TableCell className="text-xs font-sans font-medium">{n.event}</TableCell>
                            <TableCell className="text-center"><Switch checked={n.sms} onCheckedChange={v => { const notifsCopy = [...notifs]; notifsCopy[i].sms = v; setNotifs(notifsCopy); }} /></TableCell>
                            <TableCell className="text-center"><Switch checked={n.email} onCheckedChange={v => { const notifsCopy = [...notifs]; notifsCopy[i].email = v; setNotifs(notifsCopy); }} /></TableCell>
                            <TableCell className="text-center"><Switch checked={n.push} onCheckedChange={v => { const notifsCopy = [...notifs]; notifsCopy[i].push = v; setNotifs(notifsCopy); }} /></TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </CardContent>
                </Card>
              </div>
            )}
          </motion.div>
        </AnimatePresence>

        {/* Wizard footer */}
        <div className="flex items-center justify-between pt-2 border-t">
          <Button variant="outline" size="sm" className="text-xs font-sans" onClick={() => setStep(Math.max(1, step - 1))} disabled={step === 1}>
            <ChevronLeft className="h-3.5 w-3.5 mr-1" /> Back
          </Button>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" className="text-xs font-sans" onClick={() => toast({ title: "Draft saved", description: "Product configuration saved as draft." })}>
              <Save className="h-3.5 w-3.5 mr-1" /> Save as Draft
            </Button>
            {step < 6 ? (
              <Button size="sm" className="text-xs font-sans bg-primary hover:bg-primary/90" onClick={() => setStep(step + 1)}>
                Next <ChevronRight className="h-3.5 w-3.5 ml-1" />
              </Button>
            ) : (
              <Button size="sm" className="text-xs font-sans bg-success hover:bg-success/90 text-white" onClick={() => setConfirmOpen(true)}>
                <Rocket className="h-3.5 w-3.5 mr-1" /> Activate Product
              </Button>
            )}
          </div>
        </div>
      </div>

      {/* Templates Sheet */}
      <Sheet open={templatesOpen} onOpenChange={setTemplatesOpen}>
        <SheetContent className="w-full sm:max-w-md overflow-y-auto">
          <SheetHeader>
            <SheetTitle className="font-heading">Best Practice Templates</SheetTitle>
          </SheetHeader>
          <div className="space-y-3 mt-4">
            {productTemplates.map((tpl) => (
              <Card key={tpl.id} className="hover:shadow-md hover:border-accent/30 transition-all cursor-pointer" onClick={() => loadTemplate(tpl.id)}>
                <CardContent className="p-4">
                  <div className="flex items-start gap-3">
                    <span className="text-2xl">{tpl.icon}</span>
                    <div className="flex-1">
                      <h4 className="text-xs font-sans font-semibold">{tpl.name}</h4>
                      <p className="text-[10px] text-muted-foreground font-sans mt-1">{tpl.description}</p>
                      <p className="text-[9px] font-mono text-muted-foreground mt-1.5 bg-muted/50 px-2 py-1 rounded">{tpl.keyParams}</p>
                      <Button size="sm" className="mt-2 text-[10px] h-7 font-sans">Load Template</Button>
                    </div>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </SheetContent>
      </Sheet>

      {/* Confirm Activation Dialog */}
      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="font-heading">Activate Product</DialogTitle>
            <DialogDescription className="font-sans text-xs">
              Review the final configuration before activation.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2 py-3">
            {[
              ["Product Name", productName || "—"],
              ["Type", productType],
              ["Amount Range", `${formatKES(minAmount)} — ${formatKES(maxAmount)}`],
              ["Tenor", `${minTenor} — ${maxTenor} months`],
              ["Interest Rate", `${annualRate}% p.a. (${rateType})`],
              ["Method", interestMethod],
              ["Frequency", repaymentFreq],
              ["Segments", segments.join(", ")],
              ["Fees", `${fees.length} configured`],
              ["GL Mappings", `${glMappings.length} events mapped`],
              ["Approval Steps", `${enabledStages.length} stages`],
              ["Documents", `${docs.length} required`],
            ].map(([k, v]) => (
              <div key={k} className="flex justify-between text-xs py-1 border-b border-border/50">
                <span className="text-muted-foreground font-sans">{k}</span>
                <span className="font-mono font-medium">{v}</span>
              </div>
            ))}
          </div>
          <p className="text-[10px] text-destructive font-sans">This action will make the product available for origination immediately.</p>
          <DialogFooter>
            <Button variant="outline" size="sm" className="text-xs font-sans" onClick={() => setConfirmOpen(false)}>Cancel</Button>
            <Button size="sm" className="text-xs font-sans bg-success hover:bg-success/90 text-white" onClick={handleActivate} disabled={activating}>
              {activating ? <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" /> : <Rocket className="h-3.5 w-3.5 mr-1" />}
              {activating ? "Activating..." : "Confirm Activate"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
};

export default ProductConfigPage;
