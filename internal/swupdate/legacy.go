package swupdate

import (
	"fmt"

	"github.com/rmitchellscott/remarkable-go/catalog"
)

const (
	legacyManifestURL = "https://dlqathbgqp3nv.cloudfront.net/omaha/images.json"
	legacyBucketBase  = "https://dlqathbgqp3nv.cloudfront.net/omaha/"
)

var legacyPlatforms = map[string]string{
	"rm1": "reMarkable",
	"rm2": "reMarkable2",
}

func ListLegacyVersions(deviceType string) ([]OSVersionInfo, error) {
	platform, ok := legacyPlatforms[deviceType]
	if !ok {
		return nil, fmt.Errorf("legacy OS images are only available for reMarkable 1 and 2 (got %q)", deviceType)
	}

	cat, err := catalog.Load(legacyManifestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load legacy image manifest: %w", err)
	}

	entries := cat.Versions(platform)
	versions := make([]OSVersionInfo, 0, len(entries))
	for _, e := range entries {
		versions = append(versions, OSVersionInfo{
			Version:  e.Version,
			Filename: e.Key,
			Size:     e.Size,
		})
	}
	if len(versions) > 0 {
		versions[0].IsLatest = true
	}
	return versions, nil
}

func LegacyEntry(deviceType, version string) (catalog.Entry, error) {
	platform, ok := legacyPlatforms[deviceType]
	if !ok {
		return catalog.Entry{}, fmt.Errorf("legacy OS images are only available for reMarkable 1 and 2 (got %q)", deviceType)
	}

	cat, err := catalog.Load(legacyManifestURL)
	if err != nil {
		return catalog.Entry{}, fmt.Errorf("failed to load legacy image manifest: %w", err)
	}

	e, ok := cat.Lookup(platform, version)
	if !ok {
		return catalog.Entry{}, fmt.Errorf("legacy version %s not found for %s", version, deviceType)
	}
	return e, nil
}

func LegacyImageURL(key string) string {
	return legacyBucketBase + key
}
