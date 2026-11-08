package fingerprint

import (
	"bytes"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBytesAndReaderAgree(t *testing.T) {
	data := []byte("%PDF-1.4 fake invoice")
	fp := Bytes(data)
	assert.Len(t, fp, 64)
	assert.True(t, IsValidHex64(fp))
	fr, err := Reader(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, fp, fr)
	assert.NotEqual(t, fp, Bytes([]byte("%PDF-1.4 fake invoice ")))
	// sha256 of known input
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", Bytes(nil))
}

func TestNormalize(t *testing.T) {
	assert.Equal(t, "INV001", NormalizeID(" inv-001 "))
	assert.Equal(t, NormalizeID("INV 001"), NormalizeID("Inv/001"))
	assert.Equal(t, "acme", NormalizeName("ACME Ltd."))
	assert.Equal(t, NormalizeName("Acme Limited"), NormalizeName("The ACME Co"))
	assert.Equal(t, "example.com", EmailDomain("Buyer@Example.com"))
	assert.Equal(t, "", EmailDomain("nope"))
	assert.False(t, IsValidHex64("zz"))
}

func TestFieldsHashIgnoresFormatting(t *testing.T) {
	a := Fields("P051234567A", "INV-001", "P098765432B", decimal.RequireFromString("1500.5"), "kes")
	b := Fields("p051234567a", "inv 001", "P098765432B", decimal.RequireFromString("1500.50"), "KES")
	assert.Equal(t, a, b)
	c := Fields("P051234567A", "INV-002", "P098765432B", decimal.RequireFromString("1500.5"), "KES")
	assert.NotEqual(t, a, c)
}
