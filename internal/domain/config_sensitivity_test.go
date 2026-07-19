package domain

import "testing"

func TestRedactConfigMap(t *testing.T) {
	got := RedactConfigMap(
		map[string]string{"PORT": "3000", "DB": "secret-val", "X": "y"},
		map[string]string{"DB": SensitivitySecret, "PORT": SensitivityPlain},
	)
	if got["PORT"] != "3000" || got["DB"] != SecretSentinel || got["X"] != "y" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestEffectiveSensitivity(t *testing.T) {
	sec := SensitivitySecret
	pln := SensitivityPlain
	if EffectiveSensitivity("", nil) != SensitivityPlain {
		t.Fatal("new key default plain")
	}
	if EffectiveSensitivity(SensitivitySecret, nil) != SensitivitySecret {
		t.Fatal("sticky secret")
	}
	if EffectiveSensitivity(SensitivityPlain, &sec) != SensitivitySecret {
		t.Fatal("promote to secret")
	}
	if EffectiveSensitivity(SensitivitySecret, &pln) != SensitivityPlain {
		t.Fatal("explicit demote")
	}
}

func TestResolveSensitivityWinner(t *testing.T) {
	got := ResolveSensitivityWinner(
		map[string]string{"A": SensitivitySecret, "B": SensitivitySecret},
		map[string]string{"B": SensitivityPlain, "C": SensitivitySecret},
	)
	if got["A"] != SensitivitySecret || got["B"] != SensitivityPlain || got["C"] != SensitivitySecret {
		t.Fatalf("got %+v", got)
	}
}
