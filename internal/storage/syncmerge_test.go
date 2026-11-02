package storage

import (
	"reflect"
	"testing"
)

func entries(pairs ...any) []syncedEntry[string] {
	var result []syncedEntry[string]
	for i := 0; i < len(pairs); i += 3 {
		result = append(result, syncedEntry[string]{
			ID:    pairs[i].(string),
			Stamp: int64(pairs[i+1].(int)),
			Value: pairs[i+2].(string),
		})
	}
	return result
}

func mergedValues(merged []syncedEntry[string]) map[string]string {
	values := map[string]string{}
	for _, entry := range merged {
		values[entry.ID] = entry.Value
	}
	return values
}

func TestMergeKeepsEntriesOnlyOneSideHas(t *testing.T) {
	merged, removed := mergeCollections(entries("a", 1, "local"), entries("b", 1, "remote"), nil, nil)

	want := map[string]string{"a": "local", "b": "remote"}
	if got := mergedValues(merged); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %v, want none", removed)
	}
}

func TestMergeTakesTheNewerChangeOfAnEntry(t *testing.T) {
	merged, _ := mergeCollections(entries("a", 5, "local"), entries("a", 9, "remote"), nil, nil)
	if got := mergedValues(merged)["a"]; got != "remote" {
		t.Fatalf("a = %q, want remote", got)
	}

	merged, _ = mergeCollections(entries("a", 9, "local"), entries("a", 5, "remote"), nil, nil)
	if got := mergedValues(merged)["a"]; got != "local" {
		t.Fatalf("a = %q, want local", got)
	}
}

func TestMergeRemovesAnEntryDeletedAfterItsLastChange(t *testing.T) {
	merged, removed := mergeCollections(entries("a", 5, "local"), nil, nil, map[string]int64{"a": 6})

	if len(merged) != 0 {
		t.Fatalf("merged = %v, want a removed", mergedValues(merged))
	}
	if removed["a"] != 6 {
		t.Fatalf("removed = %v, want a tombstoned at 6", removed)
	}
}

func TestMergeKeepsAnEntryReaddedAfterItsRemoval(t *testing.T) {
	merged, removed := mergeCollections(entries("a", 8, "readded"), nil, nil, map[string]int64{"a": 6})

	if got := mergedValues(merged)["a"]; got != "readded" {
		t.Fatalf("a = %q, want readded", got)
	}
	if _, ok := removed["a"]; ok {
		t.Fatalf("stale tombstone kept: %v", removed)
	}
}

func TestMergeBreaksTiesTheSameWayOnBothHosts(t *testing.T) {
	first, _ := mergeCollections(entries("a", 5, "x"), entries("a", 5, "y"), nil, nil)
	second, _ := mergeCollections(entries("a", 5, "y"), entries("a", 5, "x"), nil, nil)

	if mergedValues(first)["a"] != mergedValues(second)["a"] {
		t.Fatalf("hosts disagree: %v vs %v", mergedValues(first), mergedValues(second))
	}
}

func TestEncodedCollectionIsStableAcrossOrder(t *testing.T) {
	forward, _ := encodeCollection(entries("a", 1, "x", "b", 1, "y"), map[string]int64{"c": 1, "d": 2})
	backward, _ := encodeCollection(entries("b", 1, "y", "a", 1, "x"), map[string]int64{"d": 2, "c": 1})

	if forward != backward {
		t.Fatalf("encoding depends on order:\n%s\n%s", forward, backward)
	}
}

func TestMergeKeepsEntriesSavedBeforeSyncExisted(t *testing.T) {
	unstamped := entries("a", 0, "saved before sync")

	merged, removed := mergeCollections(unstamped, unstamped, nil, nil)

	if got := mergedValues(merged)["a"]; got != "saved before sync" {
		t.Fatalf("an unstamped entry merged with itself was dropped: merged=%v removed=%v", mergedValues(merged), removed)
	}
}
