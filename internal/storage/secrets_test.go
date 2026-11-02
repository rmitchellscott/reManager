package storage

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentSecretUpdatesAreAllKept(t *testing.T) {
	ss := &SecretStore{secretsPath: filepath.Join(t.TempDir(), "secrets.json")}

	const writers = 20
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i)
			if err := ss.updateSecrets(func(secrets map[string]string) { secrets[key] = "value" }); err != nil {
				t.Errorf("updateSecrets(%s): %v", key, err)
			}
		}()
	}
	wg.Wait()

	secrets, err := ss.loadSecrets()
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets) != writers {
		t.Errorf("kept %d secrets, want %d", len(secrets), writers)
	}
}
