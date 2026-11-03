package cloudsync

import (
	"errors"
	"testing"
)

type memoryStore struct {
	values map[string]string
	sets   int
}

func (m *memoryStore) Get(key string) (string, bool) {
	value, ok := m.values[key]
	return value, ok
}

func (m *memoryStore) Set(key, value string) {
	m.values[key] = value
	m.sets++
}

type unionDomain struct {
	names    map[string]bool
	mergeErr error
}

func (d *unionDomain) SyncKey() string { return "names" }

func (d *unionDomain) ExportSync() (string, error) {
	encoded := ""
	for _, name := range []string{"a", "b", "c"} {
		if d.names[name] {
			encoded += name
		}
	}
	return encoded, nil
}

func (d *unionDomain) MergeSync(remote string) (bool, error) {
	if d.mergeErr != nil {
		return false, d.mergeErr
	}
	changed := false
	for _, r := range remote {
		if !d.names[string(r)] {
			d.names[string(r)] = true
			changed = true
		}
	}
	return changed, nil
}

func syncerOver(store KeyValueStore, onMerged func(string), domain Domain) *Syncer {
	s := NewSyncer(onMerged, domain)
	s.store = store
	return s
}

func TestFirstReconcilePublishesLocalState(t *testing.T) {
	store := &memoryStore{values: map[string]string{}}
	domain := &unionDomain{names: map[string]bool{"a": true}}

	syncerOver(store, nil, domain).reconcile("names")

	if store.values["names"] != "a" {
		t.Fatalf("published %q, want a", store.values["names"])
	}
}

func TestReconcileJoinsBothSidesAndReportsTheMerge(t *testing.T) {
	store := &memoryStore{values: map[string]string{"names": "b"}}
	domain := &unionDomain{names: map[string]bool{"a": true}}
	var merged []string

	syncerOver(store, func(key string) { merged = append(merged, key) }, domain).reconcile("names")

	if !domain.names["b"] {
		t.Fatalf("local store did not receive b")
	}
	if store.values["names"] != "ab" {
		t.Fatalf("published %q, want ab", store.values["names"])
	}
	if len(merged) != 1 {
		t.Fatalf("onMerged calls = %v, want one", merged)
	}
}

func TestReconcileDoesNotRepublishAnUnchangedValue(t *testing.T) {
	store := &memoryStore{values: map[string]string{"names": "a"}}
	domain := &unionDomain{names: map[string]bool{"a": true}}

	syncerOver(store, nil, domain).reconcile("names")

	if store.sets != 0 {
		t.Fatalf("republished an unchanged value %d times", store.sets)
	}
}

func TestFailedMergeStillPublishesLocalState(t *testing.T) {
	store := &memoryStore{values: map[string]string{"names": "corrupt"}}
	domain := &unionDomain{names: map[string]bool{"a": true}, mergeErr: errors.New("bad payload")}

	syncerOver(store, nil, domain).reconcile("names")

	if store.values["names"] != "a" {
		t.Fatalf("published %q, want the local state to replace an unreadable value", store.values["names"])
	}
}
