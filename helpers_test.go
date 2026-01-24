package simples3

import "testing"

func TestEncodePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "simple alphanumeric",
			input:    "abc123",
			expected: "abc123",
		},
		{
			name:     "unreserved characters",
			input:    "path-with_special.chars~file/key",
			expected: "path-with_special.chars~file/key",
		},
		{
			name:     "space character",
			input:    "hello world",
			expected: "hello%20world",
		},
		{
			name:     "special characters",
			input:    "hello!@#$%",
			expected: "hello%21%40%23%24%25",
		},
		{
			name:     "unicode characters",
			input:    "文件名",
			expected: "%E6%96%87%E4%BB%B6%E5%90%8D",
		},
		{
			name:     "mixed content",
			input:    "folder/文件-2024.txt",
			expected: "folder/%E6%96%87%E4%BB%B6-2024.txt",
		},
		{
			name:     "comma and parentheses",
			input:    "file(1),test.txt",
			expected: "file%281%29%2Ctest.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodePath(tt.input)
			if got != tt.expected {
				t.Errorf("encodePath() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEncodeTagsHeader(t *testing.T) {
	tests := []struct {
		name     string
		tags     map[string]string
		expected string
	}{
		{
			name:     "nil tags",
			tags:     nil,
			expected: "",
		},
		{
			name:     "empty tags",
			tags:     map[string]string{},
			expected: "",
		},
		{
			name:     "single tag",
			tags:     map[string]string{"key": "value"},
			expected: "key=value",
		},
		{
			name: "multiple tags sorted alphabetically",
			tags: map[string]string{
				"zebra": "animal",
				"apple": "fruit",
				"car":   "vehicle",
			},
			expected: "apple=fruit&car=vehicle&zebra=animal",
		},
		{
			name: "tags with special characters",
			tags: map[string]string{
				"key with space": "value with space",
				"key=equals":     "value&ampersand",
			},
			expected: "key+with+space=value+with+space&key%3Dequals=value%26ampersand",
		},
		{
			name:     "tag with empty value",
			tags:     map[string]string{"emptyvalue": ""},
			expected: "emptyvalue=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeTagsHeader(tt.tags)
			if got != tt.expected {
				t.Errorf("encodeTagsHeader() = %v, want %v", got, tt.expected)
			}
		})
	}
}
