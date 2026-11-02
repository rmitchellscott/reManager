package storage

import (
	"bytes"
	"encoding/json"
	"sort"
	"time"
)

type syncedEntry[T any] struct {
	ID    string
	Stamp int64 // Unix milliseconds of the entry's last change
	Value T
}

// Removed tombstones let a merge tell an entry the other host deleted apart from one it has never seen.
type syncedCollection[T any] struct {
	Entries []T              `json:"entries"`
	Removed map[string]int64 `json:"removed,omitempty"`
}

// Equal stamps fall back to comparing encoded values, so two hosts merging the same pair converge instead of pushing a merge back and forth.
func mergeCollections[T any](local, remote []syncedEntry[T], localRemoved, remoteRemoved map[string]int64) ([]syncedEntry[T], map[string]int64) {
	type candidate struct {
		entry   *syncedEntry[T]
		removed int64
	}
	candidates := map[string][]candidate{}
	for i := range local {
		candidates[local[i].ID] = append(candidates[local[i].ID], candidate{entry: &local[i]})
	}
	for i := range remote {
		candidates[remote[i].ID] = append(candidates[remote[i].ID], candidate{entry: &remote[i]})
	}
	// A real tombstone is always stamped after the entry it removes, so a zero stamp is one written by the unstamped-entry bug; dropping it revives the entry it wrongly removed.
	for _, removedSet := range []map[string]int64{localRemoved, remoteRemoved} {
		for id, stamp := range removedSet {
			if stamp > 0 {
				candidates[id] = append(candidates[id], candidate{removed: stamp})
			}
		}
	}

	var merged []syncedEntry[T]
	removed := map[string]int64{}
	for id, options := range candidates {
		var winner *syncedEntry[T]
		var newestRemoval int64
		tombstoned := false
		for _, option := range options {
			if option.entry == nil {
				tombstoned = true
				newestRemoval = max(newestRemoval, option.removed)
				continue
			}
			if winner == nil || entryIsNewer(*option.entry, *winner) {
				winner = option.entry
			}
		}
		// An entry saved before sync existed has a zero stamp, so only an actual tombstone may outrank it.
		if winner != nil && (!tombstoned || winner.Stamp > newestRemoval) {
			merged = append(merged, *winner)
		} else {
			removed[id] = newestRemoval
		}
	}

	sort.Slice(merged, func(i, j int) bool { return merged[i].ID < merged[j].ID })
	return merged, removed
}

func entryIsNewer[T any](candidate, current syncedEntry[T]) bool {
	if candidate.Stamp != current.Stamp {
		return candidate.Stamp > current.Stamp
	}
	candidateJSON, _ := json.Marshal(candidate.Value)
	currentJSON, _ := json.Marshal(current.Value)
	return bytes.Compare(candidateJSON, currentJSON) > 0
}

func encodeCollection[T any](entries []syncedEntry[T], removed map[string]int64) (string, error) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	collection := syncedCollection[T]{Entries: make([]T, 0, len(entries)), Removed: removed}
	for _, entry := range entries {
		collection.Entries = append(collection.Entries, entry.Value)
	}
	if len(collection.Removed) == 0 {
		collection.Removed = nil
	}
	data, err := json.Marshal(collection)
	return string(data), err
}

func decodeCollection[T any](payload string) (syncedCollection[T], error) {
	var collection syncedCollection[T]
	err := json.Unmarshal([]byte(payload), &collection)
	return collection, err
}

func nowMillis() int64 { return time.Now().UnixMilli() }

// Keeps a change strictly newer than the state it replaces even within the same millisecond, so a merge never reads it as a tie.
func nextStamp(current int64) int64 { return max(nowMillis(), current+1) }

func millisRFC3339(stamp int64) string {
	return time.UnixMilli(stamp).UTC().Format(time.RFC3339Nano)
}

// An unparseable timestamp sorts as oldest rather than failing the merge.
func rfc3339Millis(value string) int64 {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0
	}
	return parsed.UnixMilli()
}
