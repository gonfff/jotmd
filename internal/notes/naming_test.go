package notes

import "testing"

func TestFilename(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		want    string
		wantErr bool
	}{
		{name: "ASCII", title: "Release Notes", want: "release-notes.md"},
		{name: "Cyrillic", title: "СРОЧНЫЕ ЗАМЕТКИ", want: "срочные-заметки.md"},
		{name: "CJK", title: "项目计划", want: "项目计划.md"},
		{name: "emoji adjacent text", title: "Hot🔥Take", want: "hottake.md"},
		{name: "combining marks", title: "Cafe\u0301", want: "cafe\u0301.md"},
		{name: "Unicode whitespace", title: "\u00a0Hello\u2003world\u00a0", want: "hello-world.md"},
		{name: "punctuation", title: "... Hello, -- world!!! ", want: "hello-world.md"},
		{name: "markdown suffix", title: "Draft.MD", want: "draft.md"},
		{name: "one markdown suffix", title: "Draft.md.md", want: "draft-md.md"},
		{name: "empty", title: "  ", wantErr: true},
		{name: "suffix only", title: ".md", wantErr: true},
		{name: "control", title: "line\nbreak", wantErr: true},
		{name: "NUL", title: "nul\x00byte", wantErr: true},
		{name: "slash", title: "nested/name", wantErr: true},
		{name: "backslash", title: "nested\\name", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Filename(test.title)
			if test.wantErr {
				if err == nil {
					t.Fatalf("Filename(%q) = %q, nil error", test.title, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("Filename(%q) = %q, want %q", test.title, got, test.want)
			}
		})
	}
}
