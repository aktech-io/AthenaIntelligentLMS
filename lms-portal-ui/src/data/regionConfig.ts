// ─── Region / Country Config ─────────────────────────
// Minimal static config (no fake LMS portfolio data)

export interface Country {
  code: string;
  name: string;
  flag: string;
  currency: string;
  symbol: string;
  timezone: string;
  regulator: string;
  language: string;
}

export interface Holiday {
  date: string;
  name: string;
  country: string;
  recurring: boolean;
}

export const countries: Country[] = [
  {
    code: "KEN",
    name: "Kenya",
    flag: "🇰🇪",
    currency: "KES",
    symbol: "KSh",
    timezone: "Africa/Nairobi",
    regulator: "Central Bank of Kenya",
    language: "English",
  },
  {
    code: "UGA",
    name: "Uganda",
    flag: "🇺🇬",
    currency: "UGX",
    symbol: "USh",
    timezone: "Africa/Kampala",
    regulator: "Bank of Uganda",
    language: "English",
  },
  {
    code: "TZA",
    name: "Tanzania",
    flag: "🇹🇿",
    currency: "TZS",
    symbol: "TSh",
    timezone: "Africa/Dar_es_Salaam",
    regulator: "Bank of Tanzania",
    language: "Swahili",
  },
];

export const holidays: Holiday[] = [
  { date: "2026-01-01", name: "New Year's Day", country: "KEN", recurring: true },
  { date: "2026-05-01", name: "Labour Day", country: "KEN", recurring: true },
  { date: "2026-06-01", name: "Madaraka Day", country: "KEN", recurring: true },
  { date: "2026-10-20", name: "Mashujaa Day", country: "KEN", recurring: true },
  { date: "2026-12-12", name: "Jamhuri Day", country: "KEN", recurring: true },
  { date: "2026-12-25", name: "Christmas Day", country: "KEN", recurring: true },
  { date: "2026-01-26", name: "Liberation Day", country: "UGA", recurring: true },
  { date: "2026-04-26", name: "Union Day", country: "TZA", recurring: true },
];
