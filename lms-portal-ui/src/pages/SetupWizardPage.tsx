import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Check, Lock, Loader2, Building2, Globe, DollarSign, GitBranch, BookOpen, CreditCard, UserCog, ChevronRight, Upload, CheckCircle2, AlertCircle } from "lucide-react";
import { orgService, type UpdateOrgSettings } from "@/services/orgService";
import { apiGet } from "@/lib/api";
import { toast } from "@/hooks/use-toast";

const steps = [
  { id: 1, label: "Institution Identity", icon: Building2 },
  { id: 2, label: "Country Entities", icon: Globe },
  { id: 3, label: "Currencies & FX Rates", icon: DollarSign },
  { id: 4, label: "Branch Hierarchy", icon: GitBranch },
  { id: 5, label: "Chart of Accounts", icon: BookOpen },
  { id: 6, label: "Payment Rails", icon: CreditCard },
  { id: 7, label: "Admin User & Activation", icon: UserCog },
];

const availableCountries = [
  { code: "KEN", name: "Kenya", flag: "\u{1f1f0}\u{1f1ea}", currency: "KES", timezone: "Africa/Nairobi", regulator: "Central Bank of Kenya", vat: 16, wht: 15, excise: 20, rails: ["M-Pesa", "Airtel", "RTGS"] },
  { code: "UGA", name: "Uganda", flag: "\u{1f1fa}\u{1f1ec}", currency: "UGX", timezone: "Africa/Kampala", regulator: "Bank of Uganda", vat: 18, wht: 15, excise: 12, rails: ["MTN MoMo", "Airtel", "RTGS"] },
  { code: "TZA", name: "Tanzania", flag: "\u{1f1f9}\u{1f1ff}", currency: "TZS", timezone: "Africa/Dar_es_Salaam", regulator: "Bank of Tanzania", vat: 18, wht: 10, excise: 10, rails: ["M-Pesa", "RTGS"] },
  { code: "RWA", name: "Rwanda", flag: "\u{1f1f7}\u{1f1fc}", currency: "RWF", timezone: "Africa/Kigali", regulator: "National Bank of Rwanda", vat: 18, wht: 15, excise: 0, rails: ["MTN MoMo", "RTGS"] },
  { code: "GHA", name: "Ghana", flag: "\u{1f1ec}\u{1f1ed}", currency: "GHS", timezone: "Africa/Accra", regulator: "Bank of Ghana", vat: 15, wht: 8, excise: 0, rails: ["MTN MoMo", "RTGS"] },
  { code: "NGA", name: "Nigeria", flag: "\u{1f1f3}\u{1f1ec}", currency: "NGN", timezone: "Africa/Lagos", regulator: "Central Bank of Nigeria", vat: 7.5, wht: 10, excise: 0, rails: ["NIBSS", "RTGS"] },
];

const months = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];

const coaTemplates = [
  { id: "banking", icon: "\u{1f4ca}", title: "STANDARD BANKING CoA", desc: "IFRS-compliant", accounts: 120, categories: 6, best: "Banks, DFIs" },
  { id: "mfi", icon: "\u{1f91d}", title: "MICROFINANCE CoA", desc: "MIX Market-compatible", accounts: 85, categories: 0, best: "MFIs, SACCOs" },
  { id: "digital", icon: "\u{26a1}", title: "DIGITAL LENDER CoA", desc: "Minimal setup", accounts: 60, categories: 0, best: "Fintechs, BNPL" },
];

interface GLAccount {
  id: string;
  accountCode: string;
  accountName: string;
  accountType: string;
}

export default function SetupWizardPage() {
  const queryClient = useQueryClient();
  const [currentStep, setCurrentStep] = useState(1);
  const [selectedCountries, setSelectedCountries] = useState<string[]>(["KEN"]);
  const [selectedCoA, setSelectedCoA] = useState("");
  const [selectedMonth, setSelectedMonth] = useState("January");
  const [saving, setSaving] = useState(false);

  // Step 1 form fields
  const [legalName, setLegalName] = useState("Nemo Kenya Ltd");
  const [tradingName, setTradingName] = useState("Nemo");
  const [regNumber, setRegNumber] = useState("CPR/2018/123456");
  const [taxId, setTaxId] = useState("P051234567Z");
  const [institutionType, setInstitutionType] = useState("digital");
  const [regulator, setRegulator] = useState("Central Bank of Kenya");
  const [headOfficeAddress, setHeadOfficeAddress] = useState("Upper Hill, Nairobi, Kenya");

  // Step 4 branch fields
  const [branchName, setBranchName] = useState("");
  const [branchCode, setBranchCode] = useState("");
  const [branchType, setBranchType] = useState("head");
  const [branchCity, setBranchCity] = useState("");

  // Load existing org settings on mount to pre-populate
  const { data: orgSettings } = useQuery({
    queryKey: ["org", "settings"],
    queryFn: orgService.getSettings,
  });

  // Load existing branches
  const { data: branches } = useQuery({
    queryKey: ["org", "branches"],
    queryFn: orgService.listBranches,
  });

  // Load GL accounts for step 5
  const { data: glAccounts, isLoading: glLoading } = useQuery({
    queryKey: ["gl-accounts"],
    queryFn: async () => {
      const result = await apiGet<GLAccount[] | { content: GLAccount[] }>("/proxy/accounting/api/v1/accounting/accounts");
      if (result.error || !result.data) return [];
      // Handle both array and paginated response
      if (Array.isArray(result.data)) return result.data;
      if ("content" in result.data) return result.data.content;
      return [];
    },
    enabled: currentStep === 5,
  });

  // Pre-populate from existing settings
  useEffect(() => {
    if (orgSettings) {
      if (orgSettings.orgName) setTradingName(orgSettings.orgName);
      if (orgSettings.countryCode) {
        setSelectedCountries([orgSettings.countryCode]);
        const country = availableCountries.find(c => c.code === orgSettings.countryCode);
        if (country) setRegulator(country.regulator);
      }
    }
  }, [orgSettings]);

  const settingsMutation = useMutation({
    mutationFn: (data: UpdateOrgSettings) => orgService.updateSettings(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["org", "settings"] });
    },
    onError: (err: unknown) => {
      toast({
        title: "Failed to save",
        description: err instanceof Error ? err.message : "Unknown error",
        variant: "destructive",
      });
    },
  });

  const branchMutation = useMutation({
    mutationFn: orgService.createBranch,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["org", "branches"] });
      toast({ title: "Branch created", description: "Head office branch has been created." });
    },
    onError: (err: unknown) => {
      toast({
        title: "Failed to create branch",
        description: err instanceof Error ? err.message : "Unknown error",
        variant: "destructive",
      });
    },
  });

  const progress = ((currentStep - 1) / (steps.length - 1)) * 100;

  const toggleCountry = (code: string) => {
    setSelectedCountries((prev) =>
      prev.includes(code) ? prev.filter((c) => c !== code) : [...prev, code]
    );
  };

  const handleNext = async () => {
    setSaving(true);
    try {
      if (currentStep === 1) {
        // Save institution identity
        await settingsMutation.mutateAsync({ orgName: tradingName });
      } else if (currentStep === 2) {
        // Save country
        const primaryCountry = selectedCountries[0] || "KEN";
        const country = availableCountries.find(c => c.code === primaryCountry);
        await settingsMutation.mutateAsync({
          countryCode: primaryCountry,
          timezone: country?.timezone,
        });
      } else if (currentStep === 3) {
        // Save currency
        const primaryCountry = selectedCountries[0] || "KEN";
        const country = availableCountries.find(c => c.code === primaryCountry);
        await settingsMutation.mutateAsync({
          currency: country?.currency || "KES",
        });
      } else if (currentStep === 4) {
        // Create head office branch if name is filled and no branches exist
        if (branchName && (!branches || branches.length === 0)) {
          await branchMutation.mutateAsync({
            name: branchName,
            code: branchCode || "HQ-001",
            type: branchType === "head" ? "HEAD_OFFICE" : branchType === "full" ? "BRANCH" : branchType === "sub" ? "SATELLITE" : branchType === "agency" ? "AGENCY" : "BRANCH",
            city: branchCity,
            country: selectedCountries[0] || "KEN",
            status: "ACTIVE",
          });
        }
      }
      // Steps 5 and 6 are informational, no save needed
      setCurrentStep((s) => Math.min(s + 1, 7));
    } catch {
      // Error already handled via mutation onError
    } finally {
      setSaving(false);
    }
  };

  const handleActivate = async () => {
    setSaving(true);
    try {
      const primaryCountry = selectedCountries[0] || "KEN";
      const country = availableCountries.find(c => c.code === primaryCountry);
      await settingsMutation.mutateAsync({
        orgName: tradingName,
        countryCode: primaryCountry,
        currency: country?.currency || "KES",
        timezone: country?.timezone || "Africa/Nairobi",
      });
      localStorage.setItem("athena_setup_complete", "true");
      toast({ title: "Institution activated", description: "Setup is complete. Welcome to Nemo!" });
    } catch {
      // handled by mutation
    } finally {
      setSaving(false);
    }
  };

  return (
    <DashboardLayout title="Setup Wizard" subtitle="Configure your Nemo institution">
      <div className="flex gap-6">
        {/* Step sidebar */}
        <div className="w-60 shrink-0">
          <div className="mb-4">
            <Progress value={progress} className="h-2" />
            <p className="text-[10px] text-muted-foreground mt-1 font-sans">{Math.round(progress)}% complete</p>
          </div>
          <div className="space-y-1">
            {steps.map((step) => {
              const StepIcon = step.id < currentStep ? Check : step.id === currentStep ? step.icon : Lock;
              const isActive = step.id === currentStep;
              const isDone = step.id < currentStep;
              return (
                <button
                  key={step.id}
                  onClick={() => step.id <= currentStep && setCurrentStep(step.id)}
                  className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-md text-sm transition-colors ${
                    isActive ? "bg-primary text-primary-foreground" : isDone ? "text-success hover:bg-muted" : "text-muted-foreground"
                  }`}
                >
                  <StepIcon className="h-4 w-4 shrink-0" />
                  <span className="text-left">{step.label}</span>
                </button>
              );
            })}
          </div>
        </div>

        {/* Step content */}
        <div className="flex-1 min-w-0">
          {currentStep === 1 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Institution Identity</CardTitle>
                <CardDescription>Enter your institution's legal and regulatory details</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label>Legal Name</Label>
                    <Input value={legalName} onChange={(e) => setLegalName(e.target.value)} placeholder="Nemo Kenya Ltd" />
                  </div>
                  <div className="space-y-2">
                    <Label>Trading Name</Label>
                    <Input value={tradingName} onChange={(e) => setTradingName(e.target.value)} placeholder="Nemo" />
                  </div>
                  <div className="space-y-2">
                    <Label>Registration Number</Label>
                    <Input value={regNumber} onChange={(e) => setRegNumber(e.target.value)} placeholder="CPR/2018/123456" />
                  </div>
                  <div className="space-y-2">
                    <Label>Tax Identification Number</Label>
                    <Input value={taxId} onChange={(e) => setTaxId(e.target.value)} placeholder="P051234567Z" />
                  </div>
                  <div className="space-y-2">
                    <Label>Institution Type</Label>
                    <Select value={institutionType} onValueChange={setInstitutionType}>
                      <SelectTrigger><SelectValue /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value="bank">Commercial Bank</SelectItem>
                        <SelectItem value="mfi">Microfinance Institution</SelectItem>
                        <SelectItem value="digital">Digital Lender</SelectItem>
                        <SelectItem value="sacco">SACCO</SelectItem>
                        <SelectItem value="dfi">Development Finance</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>Regulator</Label>
                    <Input value={regulator} onChange={(e) => setRegulator(e.target.value)} placeholder="Central Bank of Kenya" />
                  </div>
                </div>
                <div className="space-y-2">
                  <Label>Head Office Address</Label>
                  <Input value={headOfficeAddress} onChange={(e) => setHeadOfficeAddress(e.target.value)} placeholder="Upper Hill, Nairobi, Kenya" />
                </div>
                <div className="space-y-2">
                  <Label>Logo Upload</Label>
                  <div className="border-2 border-dashed rounded-lg p-8 text-center hover:border-accent transition-colors cursor-pointer">
                    <Upload className="h-8 w-8 mx-auto text-muted-foreground mb-2" />
                    <p className="text-sm text-muted-foreground">Drag & drop or click to upload (512x512px recommended)</p>
                  </div>
                </div>
                <div className="space-y-2">
                  <Label>Fiscal Year Start Month</Label>
                  <div className="flex flex-wrap gap-2">
                    {months.map((m) => (
                      <button
                        key={m}
                        onClick={() => setSelectedMonth(m)}
                        className={`px-3 py-1.5 rounded-full text-xs font-medium transition-colors ${
                          selectedMonth === m ? "bg-primary text-primary-foreground" : "bg-muted hover:bg-muted/80 text-foreground"
                        }`}
                      >
                        {m.slice(0, 3)}
                      </button>
                    ))}
                  </div>
                </div>
              </CardContent>
            </Card>
          )}

          {currentStep === 2 && (
            <div className="space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle className="text-lg">Country Entities</CardTitle>
                  <CardDescription>Which countries do you operate in? You can add more later.</CardDescription>
                </CardHeader>
              </Card>
              <div className="grid grid-cols-1 gap-3">
                {availableCountries.map((c) => {
                  const selected = selectedCountries.includes(c.code);
                  return (
                    <Card key={c.code} className={`transition-all ${selected ? "border-accent shadow-md" : ""}`}>
                      <CardContent className="p-4">
                        <div className="flex items-start justify-between">
                          <div className="flex items-center gap-3">
                            <span className="text-2xl">{c.flag}</span>
                            <div>
                              <h3 className="font-medium font-sans">{c.name}</h3>
                              <p className="text-xs text-muted-foreground">
                                Currency: {c.currency} -- Timezone: {c.timezone}
                              </p>
                              {selected && (
                                <div className="mt-1 text-xs text-muted-foreground">
                                  Regulator: {c.regulator} -- VAT: {c.vat}% -- WHT: {c.wht}% -- Excise: {c.excise}%
                                  <br />Payment Rails: {c.rails.join(", ")}
                                </div>
                              )}
                            </div>
                          </div>
                          <div className="flex items-center gap-2">
                            {selected ? (
                              <>
                                <Badge className="bg-success/15 text-success border-success/30">SELECTED</Badge>
                                <Button size="sm" variant="ghost" onClick={() => toggleCountry(c.code)} className="text-xs">Remove</Button>
                              </>
                            ) : (
                              <Button size="sm" variant="outline" onClick={() => toggleCountry(c.code)}>Enable This Country</Button>
                            )}
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  );
                })}
              </div>
            </div>
          )}

          {currentStep === 3 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Currencies & FX Rates</CardTitle>
                <CardDescription>Configure active currencies and set initial exchange rates</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-2 gap-6">
                  <div>
                    <h4 className="text-sm font-medium mb-3 font-sans">Active Currencies</h4>
                    <div className="space-y-2">
                      {selectedCountries.map((code) => {
                        const country = availableCountries.find(c => c.code === code);
                        return country ? (
                          <div key={code} className="flex items-center gap-2 px-3 py-2 bg-muted rounded-md">
                            <span className="text-sm font-mono font-medium">{country.currency}</span>
                            <span className="text-xs text-muted-foreground">{country.name}</span>
                            <Badge variant="outline" className="text-[10px] ml-auto">Primary</Badge>
                          </div>
                        ) : null;
                      })}
                      {["USD", "EUR"].map((cur) => (
                        <div key={cur} className="flex items-center gap-2 px-3 py-2 bg-muted rounded-md">
                          <span className="text-sm font-mono font-medium">{cur}</span>
                          <span className="text-xs text-muted-foreground">
                            {cur === "USD" ? "US Dollar" : "Euro"}
                          </span>
                        </div>
                      ))}
                      <Button variant="outline" size="sm" className="w-full">+ Add Currency</Button>
                    </div>
                  </div>
                  <div>
                    <h4 className="text-sm font-medium mb-3 font-sans">Initial FX Rates (as of today)</h4>
                    <div className="space-y-3">
                      {[
                        { pair: "USD -> KES", mid: "129.50", buy: "128.80", sell: "130.20" },
                        { pair: "USD -> UGX", mid: "3,720", buy: "3,700", sell: "3,740" },
                        { pair: "USD -> TZS", mid: "2,540", buy: "2,530", sell: "2,550" },
                        { pair: "EUR -> USD", mid: "1.085", buy: "1.080", sell: "1.090" },
                      ].map((rate) => (
                        <div key={rate.pair} className="grid grid-cols-4 gap-2 items-center">
                          <span className="text-xs font-mono font-medium">{rate.pair}</span>
                          <Input defaultValue={rate.mid} className="h-8 text-xs font-mono" />
                          <Input defaultValue={rate.buy} className="h-8 text-xs font-mono" placeholder="Buy" />
                          <Input defaultValue={rate.sell} className="h-8 text-xs font-mono" placeholder="Sell" />
                        </div>
                      ))}
                    </div>
                    <div className="mt-4 flex items-center gap-4 text-xs">
                      <label className="flex items-center gap-2">
                        <input type="radio" name="rate-source" defaultChecked className="accent-primary" /> Manual entry
                      </label>
                      <label className="flex items-center gap-2">
                        <input type="radio" name="rate-source" className="accent-primary" /> Auto-fetch (API key req)
                      </label>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          )}

          {currentStep === 4 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Branch Hierarchy</CardTitle>
                <CardDescription>Build your branch tree structure</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-2 gap-6">
                  <div className="border rounded-lg p-4">
                    <h4 className="text-sm font-medium mb-3 font-sans">Existing Branches</h4>
                    {branches && branches.length > 0 ? (
                      <div className="space-y-1 text-sm font-sans">
                        {branches.map((b) => (
                          <div key={b.id} className="flex items-center gap-2 py-1.5 px-2 rounded bg-muted">
                            <span>{b.type === "HEAD_OFFICE" ? "\u{1f3db}" : "\u{1f3e6}"}</span>
                            <span className="font-medium">{b.name}</span>
                            <Badge variant="outline" className="text-[10px] ml-auto">{b.type}</Badge>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <p className="text-xs text-muted-foreground">No branches yet. Create your head office using the form.</p>
                    )}
                  </div>
                  <div className="border rounded-lg p-4">
                    <h4 className="text-sm font-medium mb-3 font-sans">
                      {branches && branches.length > 0 ? "Add Another Branch" : "Create Head Office"}
                    </h4>
                    <div className="space-y-3">
                      <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1">
                          <Label className="text-xs">Branch Name</Label>
                          <Input className="h-8 text-xs" placeholder="Nairobi HQ" value={branchName} onChange={(e) => setBranchName(e.target.value)} />
                        </div>
                        <div className="space-y-1">
                          <Label className="text-xs">Branch Code</Label>
                          <Input className="h-8 text-xs" placeholder="HQ-001" value={branchCode} onChange={(e) => setBranchCode(e.target.value)} />
                        </div>
                      </div>
                      <div className="space-y-1">
                        <Label className="text-xs">Branch Type</Label>
                        <Select value={branchType} onValueChange={setBranchType}>
                          <SelectTrigger className="h-8 text-xs"><SelectValue placeholder="Select type" /></SelectTrigger>
                          <SelectContent>
                            <SelectItem value="head">Head Office</SelectItem>
                            <SelectItem value="full">Full Branch</SelectItem>
                            <SelectItem value="sub">Sub-Branch</SelectItem>
                            <SelectItem value="agency">Agency</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div className="space-y-1">
                        <Label className="text-xs">City</Label>
                        <Input className="h-8 text-xs" placeholder="Nairobi" value={branchCity} onChange={(e) => setBranchCity(e.target.value)} />
                      </div>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          )}

          {currentStep === 5 && (
            <div className="space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle className="text-lg">Chart of Accounts</CardTitle>
                  <CardDescription>Your GL accounts are seeded by the accounting service</CardDescription>
                </CardHeader>
                <CardContent>
                  {glLoading ? (
                    <div className="flex items-center gap-2 text-muted-foreground text-sm py-4">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      Checking GL accounts...
                    </div>
                  ) : glAccounts && glAccounts.length > 0 ? (
                    <div className="space-y-4">
                      <div className="flex items-center gap-2 text-success text-sm">
                        <CheckCircle2 className="h-5 w-5" />
                        <span className="font-medium">{glAccounts.length} GL accounts found in the accounting service</span>
                      </div>
                      <div className="max-h-64 overflow-y-auto border rounded-lg">
                        <table className="w-full text-xs">
                          <thead className="bg-muted sticky top-0">
                            <tr>
                              <th className="text-left p-2 font-medium">Code</th>
                              <th className="text-left p-2 font-medium">Account Name</th>
                              <th className="text-left p-2 font-medium">Type</th>
                            </tr>
                          </thead>
                          <tbody>
                            {glAccounts.slice(0, 20).map((acct) => (
                              <tr key={acct.id} className="border-t">
                                <td className="p-2 font-mono">{acct.accountCode}</td>
                                <td className="p-2">{acct.accountName}</td>
                                <td className="p-2"><Badge variant="outline" className="text-[10px]">{acct.accountType}</Badge></td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                        {glAccounts.length > 20 && (
                          <p className="text-xs text-muted-foreground p-2 text-center">...and {glAccounts.length - 20} more accounts</p>
                        )}
                      </div>
                    </div>
                  ) : (
                    <div className="space-y-4">
                      <div className="flex items-center gap-2 text-amber-500 text-sm">
                        <AlertCircle className="h-5 w-5" />
                        <span>No GL accounts found. They will be seeded when the accounting service starts.</span>
                      </div>
                      <p className="text-xs text-muted-foreground">You can select a template to create accounts later:</p>
                      <div className="grid grid-cols-3 gap-4">
                        {coaTemplates.map((t) => (
                          <Card
                            key={t.id}
                            className={`cursor-pointer transition-all hover:shadow-md ${selectedCoA === t.id ? "border-accent shadow-md" : ""}`}
                            onClick={() => setSelectedCoA(t.id)}
                          >
                            <CardContent className="p-6 text-center">
                              <span className="text-3xl">{t.icon}</span>
                              <h3 className="font-heading text-base mt-3">{t.title}</h3>
                              <p className="text-xs text-muted-foreground mt-1">{t.desc}</p>
                              <p className="text-xs text-muted-foreground">{t.accounts} accounts</p>
                              <p className="text-xs text-muted-foreground mt-1">Best for: {t.best}</p>
                              {selectedCoA === t.id && (
                                <Badge className="mt-3 bg-success/15 text-success border-success/30">Selected</Badge>
                              )}
                            </CardContent>
                          </Card>
                        ))}
                      </div>
                    </div>
                  )}
                </CardContent>
              </Card>
            </div>
          )}

          {currentStep === 6 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Payment Rails</CardTitle>
                <CardDescription>Configure payment integrations per country (informational -- configure later in Settings)</CardDescription>
              </CardHeader>
              <CardContent>
                {selectedCountries.map((code) => {
                  const country = availableCountries.find(c => c.code === code);
                  if (!country) return null;
                  return (
                    <div key={code} className="mb-6">
                      <h4 className="text-sm font-medium mb-3">{country.flag} {country.name} Payment Rails</h4>
                      <div className="grid grid-cols-2 gap-3">
                        {country.rails.map((rail) => (
                          <Card key={rail}>
                            <CardContent className="p-4">
                              <div className="flex items-center gap-2 mb-2">
                                <span>{rail.includes("Pesa") || rail.includes("MoMo") || rail.includes("Airtel") ? "\u{1f4f1}" : "\u{1f3db}"}</span>
                                <span className="font-medium text-sm font-sans">{rail}</span>
                              </div>
                              <div className="flex items-center justify-between">
                                <Button size="sm" variant="outline" className="text-xs">Configure API Keys</Button>
                                <span className="text-[10px] flex items-center gap-1 text-muted-foreground">
                                  <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground" />
                                  Not set
                                </span>
                              </div>
                            </CardContent>
                          </Card>
                        ))}
                      </div>
                    </div>
                  );
                })}
              </CardContent>
            </Card>
          )}

          {currentStep === 7 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Admin User & Activation</CardTitle>
                <CardDescription>Review your setup and activate your institution</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="border rounded-lg p-4 bg-muted/30">
                  <h4 className="text-sm font-medium mb-2 font-sans">Setup Summary</h4>
                  <div className="grid grid-cols-2 gap-4 text-xs">
                    <div><span className="text-muted-foreground">Organization:</span> <span className="font-medium">{tradingName}</span></div>
                    <div><span className="text-muted-foreground">Countries:</span> <span className="font-medium">{selectedCountries.length}</span></div>
                    <div>
                      <span className="text-muted-foreground">Currency:</span>{" "}
                      <span className="font-medium">
                        {availableCountries.find(c => c.code === selectedCountries[0])?.currency || "KES"}
                      </span>
                    </div>
                    <div>
                      <span className="text-muted-foreground">Branches:</span>{" "}
                      <span className="font-medium">{branches?.length ?? 0}</span>
                    </div>
                    <div>
                      <span className="text-muted-foreground">GL Accounts:</span>{" "}
                      <span className="font-medium">{glAccounts?.length ?? "Pending"}</span>
                    </div>
                    <div>
                      <span className="text-muted-foreground">CoA Template:</span>{" "}
                      <span className="font-medium">{selectedCoA || "Default"}</span>
                    </div>
                  </div>
                </div>
                <Button
                  className="w-full"
                  onClick={handleActivate}
                  disabled={saving}
                >
                  {saving && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
                  Activate Institution
                </Button>
              </CardContent>
            </Card>
          )}

          {/* Navigation */}
          <div className="flex justify-between mt-6">
            <Button variant="outline" disabled={currentStep === 1} onClick={() => setCurrentStep((s) => s - 1)}>
              Back
            </Button>
            {currentStep < 7 && (
              <Button onClick={handleNext} disabled={saving}>
                {saving && <Loader2 className="h-4 w-4 animate-spin mr-1" />}
                Continue <ChevronRight className="h-4 w-4 ml-1" />
              </Button>
            )}
          </div>
        </div>
      </div>
    </DashboardLayout>
  );
}
