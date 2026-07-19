package domain

// Config sensitivity (S1 typing + S2 encryption at rest via internal/secrets).

const (
	SensitivityPlain  = "plain"
	SensitivitySecret = "secret"

	// SecretSentinel is the API/CLI placeholder for secret values on control-plane reads.
	// It must never be treated as a deployable config value.
	SecretSentinel = "***"
)

// NormalizeSensitivity returns plain or secret, or empty if invalid.
func NormalizeSensitivity(s string) string {
	switch s {
	case "", SensitivityPlain:
		return SensitivityPlain
	case SensitivitySecret:
		return SensitivitySecret
	default:
		return ""
	}
}

// IsSecret reports whether sensitivity is secret.
func IsSecret(sensitivity string) bool {
	return sensitivity == SensitivitySecret
}

// RedactConfigMap copies values, replacing secret keys with SecretSentinel.
// Keys missing from sensitivity are treated as plain (legacy / unknown).
func RedactConfigMap(values map[string]string, sensitivity map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		if IsSecret(sensitivity[k]) {
			out[k] = SecretSentinel
		} else {
			out[k] = v
		}
	}
	return out
}

// RedactConfigValue returns SecretSentinel when sensitivity is secret.
func RedactConfigValue(value, sensitivity string) string {
	if IsSecret(sensitivity) {
		return SecretSentinel
	}
	return value
}

// ResolveSensitivityWinner applies service-over-shared: service wins completely.
func ResolveSensitivityWinner(shared, service map[string]string) map[string]string {
	out := make(map[string]string, len(shared)+len(service))
	for k, s := range shared {
		out[k] = s
	}
	for k, s := range service {
		out[k] = s
	}
	return out
}

// EffectiveSensitivity chooses write sensitivity: explicit wins; else sticky existing; else plain.
// existing is "" when the key is new.
func EffectiveSensitivity(existing string, explicit *string) string {
	if explicit != nil {
		if n := NormalizeSensitivity(*explicit); n != "" {
			return n
		}
	}
	if existing == SensitivitySecret || existing == SensitivityPlain {
		return existing
	}
	return SensitivityPlain
}
