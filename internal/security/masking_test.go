package security

import "testing"

func TestMaskSecrets(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		secrets []string
		want    string
	}{
		{
			name:    "no secrets",
			text:    "hello world",
			secrets: nil,
			want:    "hello world",
		},
		{
			name:    "single secret replaced",
			text:    "Authorization: Bearer abc123xyz",
			secrets: []string{"abc123xyz"},
			want:    "Authorization: Bearer ***",
		},
		{
			name:    "multiple secrets replaced",
			text:    "user=alice&password=s3cr3t&key=tok3n",
			secrets: []string{"s3cr3t", "tok3n"},
			want:    "user=alice&password=***&key=***",
		},
		{
			name:    "secret shorter than minimum not masked",
			text:    "val=ab",
			secrets: []string{"ab"},
			want:    "val=ab",
		},
		{
			name:    "empty secret not masked",
			text:    "some text",
			secrets: []string{""},
			want:    "some text",
		},
		{
			name:    "secret appears multiple times",
			text:    "token=supersecret token=supersecret",
			secrets: []string{"supersecret"},
			want:    "token=*** token=***",
		},
		{
			name:    "partial match within word masked",
			text:    "prefix_mySuperSecret_suffix",
			secrets: []string{"mySuperSecret"},
			want:    "prefix_***_suffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskSecrets(tt.text, tt.secrets)
			if got != tt.want {
				t.Errorf("MaskSecrets(%q, %v) = %q, want %q", tt.text, tt.secrets, got, tt.want)
			}
		})
	}
}
