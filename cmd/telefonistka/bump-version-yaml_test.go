package telefonistka

import (
	"testing"
)

func TestUpdateYaml(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		yamlContent string
		address     string
		value       string
		want        string
	}{
		{
			name: "Test simple",
			yamlContent: `
tag: "16.1"
`,
			address: `.tag`,
			value:   "16.2",
			want: `
tag: "16.2"
`,
		},
		{
			name: "Test nested",
			yamlContent: `
image:
  repository: "postgres"
  tag: "16.1"
`,
			address: `.image.tag`,
			value:   "16.2",
			want: `
image:
  repository: "postgres"
  tag: "16.2"
`,
		},
		{
			name: "Test nested select",
			yamlContent: `
db:
  - name: "postgres"
    image:
      repository: "postgres"
      tag: "16.1"
`,
			address: `.db.[] | select(.name == "postgres").image.tag`,
			value:   "16.2",
			want: `
db:
  - name: "postgres"
    image:
      repository: "postgres"
      tag: "16.2"
`,
		},
		{
			name: "Test add missing",
			yamlContent: `
image:
  repository: "postgres"
`,
			address: `.image.tag`,
			value:   "16.2",
			want: `
image:
  repository: "postgres"
  tag: "16.2"
`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := updateYaml(tt.yamlContent, tt.address, tt.value)
			if err != nil {
				t.Errorf("updateYaml() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("updateYaml() got = %v, want %v", got, tt.want)
			}
		})
	}
}
