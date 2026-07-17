// Package market implements market packs: country-as-data configuration that
// makes the platform portable across markets (Nemo gap C2). A pack carries
// everything market-specific — currency, locale, timezone, phone prefix,
// holiday calendar, payment rails, credit bureaus, regulatory defaults — so
// that Kenya is the first pack rather than a hardcode, and a new market
// (Ethiopia next) is a data exercise, not a code change.
//
// Built-in packs are embedded from packs/*.yaml. Additional packs can be
// loaded at startup from the directory named by MARKET_PACK_DIR, overriding
// built-ins with the same code. The process-wide active pack is selected by
// MARKET_PACK (ISO 3166-1 alpha-2, default "KE") and read via Current().
package market

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed packs/*.yaml
var builtinFS embed.FS

// Holiday is a public holiday. Fixed-date holidays recur every year on
// MonthDay ("MM-DD"); dated holidays (e.g. Easter-derived) are listed per
// year under Dates ("YYYY-MM-DD").
type Holiday struct {
	Name     string   `yaml:"name"`
	MonthDay string   `yaml:"monthDay,omitempty"`
	Dates    []string `yaml:"dates,omitempty"`
}

// Support is the market's customer-support contact identity, used in
// notification templates.
type Support struct {
	Phone string `yaml:"phone"`
	Email string `yaml:"email"`
}

// Regulatory carries the market's regulatory defaults, consumed by the
// regulatory service when seeding a tenant profile.
type Regulatory struct {
	Regulator          string `yaml:"regulator"`          // e.g. "CBK", "NBE"
	DefaultLicenseType string `yaml:"defaultLicenseType"` // e.g. "DCP"
	ProvisioningKey    string `yaml:"provisioningKey"`    // e.g. "CBK_PG_04"
	ReportingCurrency  string `yaml:"reportingCurrency"`  // usually == Pack.Currency
}

// Pack is one market's configuration. Fields not yet consumed by services
// (Rails, KYC, Tax) are part of the skeleton: they define where per-market
// data lives as connectors and rule sets land.
type Pack struct {
	Code        string     `yaml:"code"` // ISO 3166-1 alpha-2
	Name        string     `yaml:"name"`
	Currency    string     `yaml:"currency"` // ISO 4217
	Timezone    string     `yaml:"timezone"` // IANA name
	Locale      string     `yaml:"locale"`   // BCP 47
	PhonePrefix string     `yaml:"phonePrefix"`
	Support     Support    `yaml:"support"`
	Regulatory  Regulatory `yaml:"regulatory"`
	Rails       []string   `yaml:"rails"`         // payment-rail connector ids
	CreditBureaus []string `yaml:"creditBureaus"` // bureau adapter ids
	KYCRuleSet  string     `yaml:"kycRuleSet"`    // KYC/AML rule-set id
	TaxRuleSet  string     `yaml:"taxRuleSet"`    // tax rule-set id
	Holidays    []Holiday  `yaml:"holidays"`
}

// Validate reports the first structural problem with the pack, if any.
func (p *Pack) Validate() error {
	switch {
	case len(p.Code) != 2 || p.Code != strings.ToUpper(p.Code):
		return fmt.Errorf("market pack: code %q is not ISO 3166-1 alpha-2", p.Code)
	case len(p.Currency) != 3:
		return fmt.Errorf("market pack %s: currency %q is not ISO 4217", p.Code, p.Currency)
	case p.Timezone == "":
		return fmt.Errorf("market pack %s: timezone is required", p.Code)
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return fmt.Errorf("market pack %s: unknown timezone %q", p.Code, p.Timezone)
	}
	return nil
}

// Location returns the pack's IANA location. Validate guarantees it loads.
func (p *Pack) Location() *time.Location {
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

var (
	mu       sync.RWMutex
	registry = map[string]*Pack{}

	currentOnce sync.Once
	current     *Pack
)

func init() {
	if err := loadBuiltins(); err != nil {
		// Built-in packs are compiled in; a parse failure is a build defect.
		panic(err)
	}
}

func loadBuiltins() error {
	entries, err := builtinFS.ReadDir("packs")
	if err != nil {
		return fmt.Errorf("market: read embedded packs: %w", err)
	}
	for _, e := range entries {
		raw, err := builtinFS.ReadFile("packs/" + e.Name())
		if err != nil {
			return fmt.Errorf("market: read %s: %w", e.Name(), err)
		}
		if err := registerYAML(raw, e.Name()); err != nil {
			return err
		}
	}
	return nil
}

func registerYAML(raw []byte, source string) error {
	var p Pack
	if err := yaml.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("market: parse %s: %w", source, err)
	}
	if err := p.Validate(); err != nil {
		return fmt.Errorf("market: %s: %w", source, err)
	}
	mu.Lock()
	registry[p.Code] = &p
	mu.Unlock()
	return nil
}

// LoadDir registers every *.yaml pack in dir, overriding built-ins with the
// same code. It is called automatically for MARKET_PACK_DIR but exported for
// tests and tooling.
func LoadDir(dir string) error {
	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return err
	}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("market: read %s: %w", path, err)
		}
		if err := registerYAML(raw, path); err != nil {
			return err
		}
	}
	return nil
}

// Get returns the registered pack for an ISO country code.
func Get(code string) (*Pack, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := registry[strings.ToUpper(code)]
	return p, ok
}

// Codes returns the registered pack codes, unordered.
func Codes() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for c := range registry {
		out = append(out, c)
	}
	return out
}

// Current returns the process-wide active pack, resolved once from
// MARKET_PACK_DIR (extra packs) and MARKET_PACK (selection, default "KE").
// An unknown selection is a deployment error: the process must not run with
// silently wrong market defaults, so Current panics at first use — in
// practice at service startup.
func Current() *Pack {
	currentOnce.Do(func() {
		if dir := os.Getenv("MARKET_PACK_DIR"); dir != "" {
			if err := LoadDir(dir); err != nil {
				panic(err)
			}
		}
		code := os.Getenv("MARKET_PACK")
		if code == "" {
			code = "KE"
		}
		p, ok := Get(code)
		if !ok {
			panic(fmt.Sprintf("market: MARKET_PACK=%q is not a registered pack (have %v)", code, Codes()))
		}
		current = p
	})
	return current
}

// Currency is shorthand for Current().Currency, the platform's default
// currency when a request or event carries none.
func Currency() string { return Current().Currency }

// Timezone is shorthand for Current().Timezone.
func Timezone() string { return Current().Timezone }
