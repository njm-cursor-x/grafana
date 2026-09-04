package pref

import "testing"

func TestIsValidThemeID_NDS9Themes(t *testing.T) {
	ids := []string{
		"desert",
		"autumn",
		"winterblues",
		"newyork",
		"osaka",
		"santiago",
		"london",
		"paris",
		"kyoto",
		"reykjavik",
		"mumbai",
		"rio",
		"berlin",
	}

	for _, id := range ids {
		if !IsValidThemeID(id) {
			t.Fatalf("expected theme id %q to be valid after themes_generated.go regeneration", id)
		}
		theme := GetThemeByID(id)
		if theme == nil {
			t.Fatalf("expected GetThemeByID(%q) to return a theme", id)
		}
		if !theme.IsExtra {
			t.Fatalf("expected theme %q to be marked IsExtra", id)
		}
	}
}

func TestIsValidThemeID_rejectsUnknown(t *testing.T) {
	if IsValidThemeID("not-a-real-theme") {
		t.Fatal("expected unknown theme id to be invalid")
	}
}
