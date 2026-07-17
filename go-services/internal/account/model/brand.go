package model

import (
	"fmt"
	"regexp"
	"strings"
)

// BrandPack is a tenant's white-label identity (Nemo C4): the runtime theme
// the portal and mobile app render. Colors are token→hex; token names follow
// the app's semantic tokens (background, surface, primary, accent, positive,
// negative, foreground, muted).
type BrandPack struct {
	AppName string            `json:"appName"`
	Tagline string            `json:"tagline,omitempty"`
	LogoURL string            `json:"logoUrl,omitempty"`
	Colors  map[string]string `json:"colors"`
	Assets  map[string]string `json:"assets,omitempty"` // named asset → media ref / URL
}

var hexColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

// Validate reports the first structural problem with the pack.
func (b *BrandPack) Validate() error {
	if strings.TrimSpace(b.AppName) == "" {
		return fmt.Errorf("brand pack: appName is required")
	}
	for token, hex := range b.Colors {
		if !hexColor.MatchString(hex) {
			return fmt.Errorf("brand pack: color %q = %q is not a hex color", token, hex)
		}
	}
	return nil
}

// DefaultBrand is the platform (Nemo deep-water) brand, returned for tenants
// with no brand pack of their own. Values mirror the concept deck and the
// NemoWallet app theme.
func DefaultBrand() BrandPack {
	return BrandPack{
		AppName: "NemoWallet",
		Tagline: "Neobank in a Box",
		Colors: map[string]string{
			"background": "#06222B", // abyss
			"surface":    "#0C3541", // water
			"primary":    "#FF6A3D", // coral
			"primaryAlt": "#E04E22", // coral-deep
			"accent":     "#5BC4B4", // aqua
			"positive":   "#5BC4B4",
			"negative":   "#FF7A6B",
			"foreground": "#F2F7F5", // stripe
			"muted":      "#8FAEAB", // glass
		},
	}
}
