package cmd

import (
	"reflect"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "simple args",
			input: "sh -c sleep infinity",
			want:  []string{"sh", "-c", "sleep", "infinity"},
		},
		{
			name:  "single quoted",
			input: "echo 'hello world'",
			want:  []string{"echo", "hello world"},
		},
		{
			name:  "double quoted",
			input: `echo "hello world"`,
			want:  []string{"echo", "hello world"},
		},
		{
			name:  "mixed quotes",
			input: `sh -c 'echo "test"'`,
			want:  []string{"sh", "-c", `echo "test"`},
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:    "unclosed single quote",
			input:   `echo 'hello`,
			wantErr: true,
		},
		{
			name:    "unclosed double quote",
			input:   `echo "hello`,
			wantErr: true,
		},
		{
			name:  "file paths",
			input: "cat /etc/os-release",
			want:  []string{"cat", "/etc/os-release"},
		},
		{
			name:  "multiple spaces",
			input: "echo   hello   world",
			want:  []string{"echo", "hello", "world"},
		},
		{
			name:  "single arg",
			input: "sleep",
			want:  []string{"sleep"},
		},
		{
			name:  "args with equals",
			input: "KEY=value another=value",
			want:  []string{"KEY=value", "another=value"},
		},
		{
			name:  "quoted empty string",
			input: `echo ""`,
			want:  []string{"echo", ""},
		},
		{
			name:  "nested different quotes",
			input: `sh -c "echo 'test'"`,
			want:  []string{"sh", "-c", "echo 'test'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}
