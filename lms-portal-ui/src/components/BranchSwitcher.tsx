import { useState } from "react";
import { Building2, ChevronDown, Globe } from "lucide-react";
import { countries } from "@/data/regionConfig";

const branches: { id: string; name: string; currency: string; countryCode: string; type: string }[] = [];
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";

export type BranchSelection = {
  id: string;
  name: string;
  currency: string;
  countryFlag: string;
} | null; // null = ALL BRANCHES

interface BranchSwitcherProps {
  currentBranch: BranchSelection;
  onBranchChange: (branch: BranchSelection) => void;
}

export function BranchSwitcher({ currentBranch, onBranchChange }: BranchSwitcherProps) {
  const [open, setOpen] = useState(false);

  const branchesByCountry = countries.map((country) => ({
    country,
    branches: branches.filter((b) => b.countryCode === country.code),
  }));

  const pillLabel = currentBranch
    ? `${currentBranch.countryFlag} ${currentBranch.name} – ${currentBranch.currency}`
    : "🌍 All Branches";

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button className="flex items-center gap-2 px-3 py-1.5 rounded-full bg-muted hover:bg-muted/80 border border-border text-xs font-medium transition-colors">
          {currentBranch ? (
            <Building2 className="h-3.5 w-3.5 text-muted-foreground" />
          ) : (
            <Globe className="h-3.5 w-3.5 text-muted-foreground" />
          )}
          <span>{pillLabel}</span>
          <ChevronDown className="h-3 w-3 text-muted-foreground" />
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-80 p-0" align="start">
        <div className="p-3 border-b">
          <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Branch Context</p>
        </div>
        <div className="p-1">
          <button
            onClick={() => { onBranchChange(null); setOpen(false); }}
            className={`w-full flex items-center gap-2 px-3 py-2 rounded-md text-sm hover:bg-muted transition-colors ${!currentBranch ? "bg-muted font-medium" : ""}`}
          >
            <Globe className="h-4 w-4 text-muted-foreground" />
            <span>ALL BRANCHES (Consolidated)</span>
          </button>
        </div>
        <div className="max-h-64 overflow-y-auto">
          {branchesByCountry.map(({ country, branches: cBranches }) => (
            <div key={country.code} className="px-1 pb-1">
              <div className="px-3 py-1.5 text-[11px] font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-1.5">
                <span>{country.flag}</span>
                <span>{country.name}</span>
              </div>
              {cBranches.map((branch) => (
                <button
                  key={branch.id}
                  onClick={() => {
                    onBranchChange({
                      id: branch.id,
                      name: branch.name,
                      currency: branch.currency,
                      countryFlag: country.flag,
                    });
                    setOpen(false);
                  }}
                  className={`w-full flex items-center gap-2 px-3 py-1.5 rounded-md text-sm hover:bg-muted transition-colors ${currentBranch?.id === branch.id ? "bg-muted font-medium" : ""}`}
                >
                  <span className="text-xs">
                    {branch.type === "HEAD_OFFICE" ? "🏛" : branch.type === "SUB_BRANCH" ? "🏪" : "🏦"}
                  </span>
                  <span className="flex-1 text-left">{branch.name}</span>
                  {branch.type === "HEAD_OFFICE" && (
                    <span className="text-[11px] px-1.5 py-0.5 rounded bg-accent/20 text-accent-foreground font-medium">HEAD</span>
                  )}
                </button>
              ))}
            </div>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}
