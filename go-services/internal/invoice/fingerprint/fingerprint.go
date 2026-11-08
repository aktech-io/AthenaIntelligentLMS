// Package fingerprint computes the document fingerprint (sha256 of the PDF
// bytes) and the canonical fields hash used for duplicate detection
// (SPEC §2, §3.1).
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
)

// Bytes returns the lowercase hex sha256 of data.
func Bytes(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

// Reader streams r through sha256.
func Reader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

var nonAlnum = regexp.MustCompile(`[^A-Z0-9]+`)

// NormalizeID upper-cases and strips everything but letters and digits, so
// "inv-001", "INV 001" and "Inv/001" collide as intended.
func NormalizeID(s string) string {
	return nonAlnum.ReplaceAllString(strings.ToUpper(strings.TrimSpace(s)), "")
}

// NormalizeName lower-cases, collapses whitespace and drops common legal
// suffixes so "ACME Ltd." and "Acme Limited" match.
func NormalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = regexp.MustCompile(`[^a-z0-9 ]+`).ReplaceAllString(s, " ")
	fields := strings.Fields(s)
	var out []string
	for _, f := range fields {
		switch f {
		case "ltd", "limited", "llc", "inc", "plc", "co", "company", "corp", "corporation", "the":
			continue
		}
		out = append(out, f)
	}
	return strings.Join(out, " ")
}

// EmailDomain returns the lower-cased domain part of an email, or "".
func EmailDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[at+1:]))
}

// IsValidHex64 reports whether s is a 64-char lowercase/uppercase hex string.
func IsValidHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// Fields hashes the canonical identity of an invoice: seller tax id,
// normalised invoice number, buyer tax id, total and currency.
func Fields(sellerTaxID, invoiceNumber, buyerTaxID string, total decimal.Decimal, currency string) string {
	parts := []string{
		NormalizeID(sellerTaxID),
		NormalizeID(invoiceNumber),
		NormalizeID(buyerTaxID),
		total.StringFixed(2),
		strings.ToUpper(strings.TrimSpace(currency)),
	}
	return Bytes([]byte(strings.Join(parts, "|")))
}
