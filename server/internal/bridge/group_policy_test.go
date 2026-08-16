package bridge

import (
	"testing"
)

func TestGroupPolicyDefaultsToActuallyPinnedGroups(t *testing.T) {
	policy := DefaultGroupPolicy()
	if policy.Mode() != GroupModePinned {
		t.Fatalf("unexpected default mode: %q", policy.Mode())
	}
	if !policy.Includes("Any Group", true) {
		t.Fatal("actually pinned group was excluded")
	}
	if policy.Includes("Any Group", false) {
		t.Fatal("unpinned group was included by default")
	}
}

func TestGroupPolicyAllowlistUsesNormalizedExactNamesWithoutPinning(t *testing.T) {
	policy, err := GroupPolicyFromJSON(GroupModePinned, `["  Project   Updates  "]`)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Allows("project updates") || !policy.Includes("PROJECT UPDATES", false) {
		t.Fatal("normalized allowlisted group was excluded")
	}
	if policy.Includes("Project Updates Archive", false) {
		t.Fatal("non-exact group name was included")
	}
	if policy.Mode() != GroupModePinned {
		t.Fatalf("allowlist changed mode: %q", policy.Mode())
	}
}

func TestGroupPolicyAllIncludesUnpinnedGroups(t *testing.T) {
	policy, err := GroupPolicyFromJSON(GroupModeAll, "[]")
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Includes("Any Group", false) {
		t.Fatal("all mode excluded unpinned group")
	}
}

func TestGroupPolicyRejectsInvalidEnvironmentValues(t *testing.T) {
	if _, err := GroupPolicyFromJSON("unknown", "[]"); err == nil {
		t.Fatal("invalid mode was accepted")
	}
	if _, err := GroupPolicyFromJSON(GroupModePinned, `{"name":"Project"}`); err == nil {
		t.Fatal("non-array allowlist was accepted")
	}
	if _, err := GroupPolicyFromJSON(GroupModePinned, "null"); err == nil {
		t.Fatal("non-array null allowlist was accepted")
	}
}

func TestPublicBaseURLRequiresHTTPSExceptLocalDevelopment(t *testing.T) {
	if _, err := ValidatePublicBaseURL("http://bridge.example.test", false); err == nil {
		t.Fatal("public HTTP URL was accepted")
	}
	if _, err := ValidatePublicBaseURL("http://localhost:8080", true); err != nil {
		t.Fatalf("local development URL was rejected: %v", err)
	}
	if _, err := ValidatePublicBaseURL("http://bridge.example.test", true); err == nil {
		t.Fatal("non-local HTTP URL was accepted in development")
	}
	if _, err := ValidatePublicBaseURL("https://bridge.example.test/path", false); err == nil {
		t.Fatal("URL with a path was accepted")
	}
	got, err := ValidatePublicBaseURL("https://bridge.example.test/", false)
	if err != nil || got != "https://bridge.example.test" {
		t.Fatalf("unexpected canonical URL %q, err=%v", got, err)
	}
}
