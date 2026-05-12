package swupdate

import (
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"time"

	"reManager/internal/httputil"
	"reManager/internal/version"
)

const bucketURL = "https://remarkable-software.s3.us-east-2.amazonaws.com/"

var imagePattern = regexp.MustCompile(`^remarkable-production-image-(.+)-(.+)-public\.swu$`)

var deviceFileNames = map[string]string{
	"rm1":  "rm1",
	"rm2":  "rm2",
	"rmpp": "ferrari",
	"rmppmove": "chiappa",
	"rmppure":  "tatsu",
}

type OSVersionInfo struct {
	Version     string `json:"version"`
	IsLatest    bool   `json:"isLatest"`
	IsInstalled bool   `json:"isInstalled"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
}

type listBucketResult struct {
	XMLName               xml.Name       `xml:"ListBucketResult"`
	Contents              []bucketObject `xml:"Contents"`
	IsTruncated           bool           `xml:"IsTruncated"`
	NextContinuationToken string         `xml:"NextContinuationToken"`
}

type bucketObject struct {
	Key  string `xml:"Key"`
	Size int64  `xml:"Size"`
}

func ListVersions(deviceType string, installedVersions []string) ([]OSVersionInfo, error) {
	deviceName, ok := deviceFileNames[deviceType]
	if !ok {
		return nil, fmt.Errorf("unknown device type: %s", deviceType)
	}

	objects, err := listBucket()
	if err != nil {
		return nil, fmt.Errorf("failed to list S3 bucket: %w", err)
	}

	installedSet := make(map[string]bool)
	for _, v := range installedVersions {
		installedSet[v] = true
	}

	var versions []OSVersionInfo
	for _, obj := range objects {
		matches := imagePattern.FindStringSubmatch(obj.Key)
		if matches == nil {
			continue
		}

		version := matches[1]
		device := matches[2]

		if device != deviceName {
			continue
		}

		if !isAbove322(version) {
			continue
		}

		versions = append(versions, OSVersionInfo{
			Version:     version,
			IsInstalled: installedSet[version],
			Filename:    obj.Key,
			Size:        obj.Size,
		})
	}

	sort.Slice(versions, func(i, j int) bool {
		return version.Compare(versions[i].Version, versions[j].Version) > 0
	})

	if len(versions) > 0 {
		versions[0].IsLatest = true
	}

	return versions, nil
}

func listBucket() ([]bucketObject, error) {
	var allObjects []bucketObject
	continuationToken := ""

	for {
		url := bucketURL + "?list-type=2"
		if continuationToken != "" {
			url += "&continuation-token=" + continuationToken
		}

		resp, err := httputil.NewClient(30 * time.Second).Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("S3 returned status %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		var result listBucketResult
		if err := xml.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse S3 XML: %w", err)
		}

		allObjects = append(allObjects, result.Contents...)

		if !result.IsTruncated {
			break
		}
		continuationToken = result.NextContinuationToken
	}

	return allObjects, nil
}

func isAbove322(v string) bool {
	return version.Compare(v, "3.22") > 0
}
