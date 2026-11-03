package vellum

import (
	"reflect"
	"testing"
)

func TestParseWorldPackagesStripsTagsConstraintsAndPins(t *testing.T) {
	world := "appload\nmount-utils@vellum\nremarkable-os=3.27.3.0-r0\ntrmnl-remarkable><Q1xwJwlPBZaSE6hGSsi5vQlD20Gbs=\nxovi~1.2\nvellum>=0.3\n\n"
	want := []string{"appload", "mount-utils", "remarkable-os", "trmnl-remarkable", "xovi", "vellum"}
	if got := parseWorldPackages(world); !reflect.DeepEqual(got, want) {
		t.Errorf("parseWorldPackages = %v, want %v", got, want)
	}
}
