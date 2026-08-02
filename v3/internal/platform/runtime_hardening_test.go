package platform

import (
	"reflect"
	"testing"
)

func TestLineProbeEndpointAcceptsRootAndEmbyBasePaths(t *testing.T) {
	for input, expected := range map[string]string{
		"http://emby:8096":           "http://emby:8096/emby/System/Info/Public",
		"https://media.example/":     "https://media.example/emby/System/Info/Public",
		"https://media.example/emby": "https://media.example/emby/System/Info/Public",
	} {
		if actual := lineProbeEndpoint(input); actual != expected {
			t.Fatalf("lineProbeEndpoint(%q)=%q expected %q", input, actual, expected)
		}
	}
}

func TestManagedFolderMergePreservesExternalFoldersAndRemovesRevokedGrants(t *testing.T) {
	baseline := []string{"manual-library"}
	current := []string{"manual-library", "externally-added", "managed-old"}
	lastManaged := []string{"managed-old"}
	managedNow := []string{"managed-new"}
	desired := mergeFolderSets(baseline, subtractFolderSet(current, lastManaged), managedNow)
	expected := []string{"externally-added", "managed-new", "manual-library"}
	if !reflect.DeepEqual(desired, expected) {
		t.Fatalf("desired folders=%v expected=%v", desired, expected)
	}
}
