package version

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.4.2", "1.4.1", 1},
		{"1.4.1", "1.4.2", -1},
		{"1.4.0", "1.4.0", 0},
		{"1.10.0", "1.9.0", 1},
		{"3.27.3.0", "3.27", 1},
		{"3.26", "3.27", -1},

		{"1.4.2-r1", "1.4.2-r0", 1},
		{"1.4.2-r0", "1.4.2", 0},
		{"1.4.2-r2", "1.4.3-r0", -1},

		{"0.0.11_pre2", "0.0.11_pre1", 1},
		{"0.0.11_pre4", "0.0.11_pre3", 1},
		{"0.0.11_pre10", "0.0.11_pre9", 1},
		{"0.0.11_pre1", "0.0.11", -1},
		{"0.0.11_git4", "0.0.11", 1},
		{"0.0.11_pre4_git4", "0.0.11_pre4", 1},
		{"0.0.11_pre4_git4", "0.0.11_pre3", 1},
		{"0.0.11_pre4_git4", "0.0.11", -1},

		{"1.0_alpha1", "1.0_beta1", -1},
		{"1.0_beta1", "1.0_pre1", -1},
		{"1.0_pre1", "1.0_rc1", -1},
		{"1.0_rc1", "1.0", -1},
		{"1.0", "1.0_p1", -1},

		{"1.0a", "1.0", 1},
		{"1.0a", "1.0b", -1},

		{"v1.4.2", "1.4.2", 0},
	}

	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := Compare(c.b, c.a); got != -c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.b, c.a, got, -c.want)
		}
	}
}
