package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// F4: exponential PIN lockout — base at the threshold, doubling per further
// failure, capped at max.
func TestPinLockoutDuration(t *testing.T) {
	base := 30 * time.Second
	max := 15 * time.Minute

	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{5, 30 * time.Second},  // at threshold
		{6, 60 * time.Second},  // doubles
		{7, 120 * time.Second}, // doubles again
		{9, 480 * time.Second},
		{10, max}, // 960s capped at 900s
		{50, max}, // stays capped, no overflow
		{4, 30 * time.Second}, // below threshold clamps to base (defensive)
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, pinLockoutDuration(tc.attempts, 5, base, max),
			"attempts=%d", tc.attempts)
	}
}

func TestValidatePinFormat(t *testing.T) {
	assert.NoError(t, validatePinFormat("1234"))
	assert.NoError(t, validatePinFormat("123456"))
	assert.Error(t, validatePinFormat("123"))     // too short
	assert.Error(t, validatePinFormat("1234567")) // too long
	assert.Error(t, validatePinFormat("12a4"))    // non-digit
	assert.Error(t, validatePinFormat(""))
}
