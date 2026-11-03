package main

import (
	"encoding/json"
	"testing"
)

func TestNewestAPKReleaseSkipsReleasesWithoutAnAPK(t *testing.T) {
	var releases []gitHubRelease
	payload := `[
		{"tag_name": "v2.1.0", "html_url": "https://github.com/rmitchellscott/remanager/releases/tag/v2.1.0", "assets": [{"name": "reManager-macos-universal.zip"}]},
		{"tag_name": "v2.2.0-beta.1", "prerelease": true, "assets": [{"name": "reManager-2.2.0-beta.1.apk"}]},
		{"tag_name": "v2.3.0", "draft": true, "assets": [{"name": "reManager-2.3.0.apk"}]},
		{"tag_name": "v2.0.1", "html_url": "https://github.com/rmitchellscott/remanager/releases/tag/v2.0.1", "assets": [{"name": "reManager-2.0.1.APK"}]},
		{"tag_name": "v2.0.0", "assets": [{"name": "reManager-2.0.0.apk"}, {"name": "reManager-windows-amd64.zip"}]}
	]`
	if err := json.Unmarshal([]byte(payload), &releases); err != nil {
		t.Fatal(err)
	}

	newest, found := newestAPKRelease(releases)
	if !found {
		t.Fatal("found no release with an APK")
	}
	if newest.TagName != "v2.0.1" {
		t.Errorf("newest APK release = %s, want v2.0.1", newest.TagName)
	}
	if newest.HTMLURL != "https://github.com/rmitchellscott/remanager/releases/tag/v2.0.1" {
		t.Errorf("release URL = %s", newest.HTMLURL)
	}
}

func TestNewestAPKReleaseComparesVersionsNotListOrder(t *testing.T) {
	apk := []struct {
		Name string `json:"name"`
	}{{Name: "reManager.apk"}}
	releases := []gitHubRelease{
		{TagName: "v2.0.9", Assets: apk},
		{TagName: "v2.0.10", Assets: apk},
	}

	newest, _ := newestAPKRelease(releases)
	if newest.TagName != "v2.0.10" {
		t.Errorf("newest APK release = %s, want v2.0.10", newest.TagName)
	}
}

func TestNewestAPKReleaseWithNoAPKs(t *testing.T) {
	if _, found := newestAPKRelease([]gitHubRelease{{TagName: "v2.1.0"}}); found {
		t.Error("found a release with no APK")
	}
}
