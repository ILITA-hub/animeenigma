package service

import "testing"

func TestIsValidTimezone(t *testing.T) {
	valid := []string{"UTC", "Europe/Moscow", "Asia/Tokyo", "America/New_York", "Asia/Kolkata"}
	for _, tz := range valid {
		if !IsValidTimezone(tz) {
			t.Errorf("expected %q to be valid", tz)
		}
	}

	invalid := []string{"", "Local", "Not/AZone", "Moscow", "UTC+3", "<script>"}
	for _, tz := range invalid {
		if IsValidTimezone(tz) {
			t.Errorf("expected %q to be invalid", tz)
		}
	}
}
