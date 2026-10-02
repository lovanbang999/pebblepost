package security

import "strings"

const maskPlaceholder = "***"

// minSecretLen is the minimum number of characters a secret must have before
// it is eligible for masking. Very short values (e.g. "0", "no") are too
// common in plain text and would cause false-positive redactions.
const minSecretLen = 3

// MaskSecrets replaces every occurrence of each secret value inside text with
// "***". Values shorter than minSecretLen characters are skipped.
// The function is deterministic: all replacements happen in the order provided.
func MaskSecrets(text string, secrets []string) string {
	for _, s := range secrets {
		if len(s) < minSecretLen {
			continue
		}
		text = strings.ReplaceAll(text, s, maskPlaceholder)
	}
	return text
}
