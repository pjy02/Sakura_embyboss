package identity

import "testing"

func TestDynamicSettingSchemas(t *testing.T) {
	for _, test := range []struct {
		key, valueType, raw string
		valid               bool
	}{
		{"auth.local_registration_enabled", "boolean", "true", true},
		{"auth.local_registration_enabled", "string", `"true"`, false},
		{"auth.session_ttl_hours", "integer", "168", true},
		{"auth.session_ttl_hours", "integer", "0", false},
		{"auth.session_ttl_hours", "integer", "8761", false},
	} {
		if actual := validSettingValue(test.key, []byte(test.raw), test.valueType); actual != test.valid {
			t.Fatalf("%s %s %s valid=%v, expected %v", test.key, test.valueType, test.raw, actual, test.valid)
		}
	}
}
