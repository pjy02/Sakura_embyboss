package platform

import (
	"testing"

	"github.com/google/uuid"
)

func TestDeviceAllowlistPrecedesDenyAndSuppressesAutomaticRiskAction(t *testing.T) {
	observation := playbackObservation{Session: embyPlaybackSession{Client: "Emby Web", DeviceName: "Living Room"}}
	rules := []DeviceRule{
		{ID: uuid.New(), Decision: "allow", MatchField: "client_name", MatchOperator: "exact", MatchValue: "Emby Web", Enabled: true},
		{ID: uuid.New(), Decision: "deny", MatchField: "device_name", MatchOperator: "contains", MatchValue: "Living", Enabled: true, Action: "disable_user"},
	}
	decision, matched := evaluateDeviceRules(rules, observation)
	if decision != "allowed" || matched == nil || matched.Decision != "allow" {
		t.Fatalf("decision=%s matched=%+v", decision, matched)
	}
	risk := RiskRule{RuleType: "custom", Condition: map[string]any{"field": "device_name", "operator": "contains", "value": "Living"}}
	if ok, _ := evaluateRiskRule(risk, observation, 1); !ok {
		t.Fatal("custom rule should still be observable for an allowlisted device")
	}
}

func TestRiskRuleValidationAndEvaluation(t *testing.T) {
	if validRiskCondition("custom", map[string]any{"field": "client_name", "operator": "regex", "value": "["}) {
		t.Fatal("invalid regular expression was accepted")
	}
	observation := playbackObservation{Session: embyPlaybackSession{UserID: "u1"}, Bitrate: 20_000_000, Transcoding: true}
	if matched, evidence := evaluateRiskRule(RiskRule{RuleType: "bitrate", Condition: map[string]any{"minimum_bitrate": float64(10_000_000)}}, observation, 1); !matched || evidence["bitrate"] != int64(20_000_000) {
		t.Fatalf("bitrate rule failed: matched=%v evidence=%+v", matched, evidence)
	}
	if matched, _ := evaluateRiskRule(RiskRule{RuleType: "concurrent_streams", Condition: map[string]any{"threshold": float64(3)}}, observation, 2); matched {
		t.Fatal("concurrency rule matched below threshold")
	}
}
