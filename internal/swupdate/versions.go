package swupdate

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const bucketURL = "https://remarkable-software.s3.us-east-2.amazonaws.com/"

var imagePattern = regexp.MustCompile(`^remarkable-production-image-(.+)-(.+)-public\.swu$`)

var deviceFileNames = map[string]string{
	"rm1":  "rm1",
	"rm2":  "rm2",
	"rmpp": "ferrari",
	"rmppmove": "chiappa",
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
		return compareVersions(versions[i].Version, versions[j].Version) > 0
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

		resp, err := http.Get(url)
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

func isAbove322(version string) bool {
	return compareVersions(version, "3.22") > 0
}

func compareVersions(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	maxLen := len(partsA)
	if len(partsB) > maxLen {
		maxLen = len(partsB)
	}

	for i := 0; i < maxLen; i++ {
		var na, nb int
		if i < len(partsA) {
			na, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			nb, _ = strconv.Atoi(partsB[i])
		}
		if na != nb {
			if na > nb {
				return 1
			}
			return -1
		}
	}
	return 0
}
