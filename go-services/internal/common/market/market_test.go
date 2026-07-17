package market

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinKenyaPack(t *testing.T) {
	p, ok := Get("KE")
	if !ok {
		t.Fatal("built-in KE pack not registered")
	}
	if p.Currency != "KES" {
		t.Errorf("KE currency = %q, want KES", p.Currency)
	}
	if p.Timezone != "Africa/Nairobi" {
		t.Errorf("KE timezone = %q, want Africa/Nairobi", p.Timezone)
	}
	if p.Regulatory.Regulator != "CBK" || p.Regulatory.ProvisioningKey != "CBK_PG_04" {
		t.Errorf("KE regulatory defaults wrong: %+v", p.Regulatory)
	}
	if len(p.Holidays) == 0 {
		t.Error("KE pack has no holiday calendar")
	}
	if err := p.Validate(); err != nil {
		t.Errorf("KE pack invalid: %v", err)
	}
}

func TestCurrentDefaultsToKenya(t *testing.T) {
	// MARKET_PACK is unset in the test environment.
	if got := Current().Code; got != "KE" {
		t.Errorf("Current().Code = %q, want KE", got)
	}
	if Currency() != "KES" {
		t.Errorf("Currency() = %q, want KES", Currency())
	}
	if Timezone() != "Africa/Nairobi" {
		t.Errorf("Timezone() = %q, want Africa/Nairobi", Timezone())
	}
}

// TestLoadDirNewMarket proves the C2 claim: a new market is a data file, not
// a code change.
func TestLoadDirNewMarket(t *testing.T) {
	dir := t.TempDir()
	et := `
code: ET
name: Ethiopia
currency: ETB
timezone: Africa/Addis_Ababa
locale: am-ET
phonePrefix: "+251"
regulatory:
  regulator: NBE
  reportingCurrency: ETB
`
	if err := os.WriteFile(filepath.Join(dir, "et.yaml"), []byte(et), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	p, ok := Get("ET")
	if !ok {
		t.Fatal("ET pack not registered after LoadDir")
	}
	if p.Currency != "ETB" || p.Regulatory.Regulator != "NBE" {
		t.Errorf("ET pack fields wrong: %+v", p)
	}
}

func TestValidateRejectsBadPacks(t *testing.T) {
	cases := []Pack{
		{Code: "kenya", Currency: "KES", Timezone: "Africa/Nairobi"},
		{Code: "KE", Currency: "KE", Timezone: "Africa/Nairobi"},
		{Code: "KE", Currency: "KES", Timezone: ""},
		{Code: "KE", Currency: "KES", Timezone: "Africa/Atlantis"},
	}
	for i, p := range cases {
		if err := p.Validate(); err == nil {
			t.Errorf("case %d: Validate() accepted invalid pack %+v", i, p)
		}
	}
}

func TestBuiltinEthiopiaPack(t *testing.T) {
	p, ok := Get("ET")
	if !ok {
		t.Fatal("built-in ET pack not registered")
	}
	if p.Currency != "ETB" || p.Timezone != "Africa/Addis_Ababa" || p.Regulatory.Regulator != "NBE" {
		t.Errorf("ET pack fields wrong: %+v", p)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("ET pack invalid: %v", err)
	}
}
