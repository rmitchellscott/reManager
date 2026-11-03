package support

import "testing"

func TestGetSupportURLPrecedence(t *testing.T) {
	tests := []struct {
		name          string
		buildURL      string
		envURL        string
		want          string
		wantAvailable bool
	}{
		{name: "source build", want: "", wantAvailable: false},
		{name: "official build", buildURL: "https://build.example", want: "https://build.example", wantAvailable: true},
		{name: "env on source build", envURL: "http://localhost:8080", want: "http://localhost:8080", wantAvailable: true},
		{name: "env over build", buildURL: "https://build.example", envURL: "http://localhost:8080", want: "http://localhost:8080", wantAvailable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved := supportURL
			t.Cleanup(func() { supportURL = saved })
			supportURL = tt.buildURL
			t.Setenv("REMANAGER_SUPPORT_URL", tt.envURL)

			if got := GetSupportURL(); got != tt.want {
				t.Errorf("GetSupportURL() = %q, want %q", got, tt.want)
			}
			if got := UploadAvailable(); got != tt.wantAvailable {
				t.Errorf("UploadAvailable() = %v, want %v", got, tt.wantAvailable)
			}
		})
	}
}
