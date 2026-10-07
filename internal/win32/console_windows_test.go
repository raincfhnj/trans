//go:build windows

package win32

import "testing"

// The line the child program is started with is the only place its arguments
// exist: whatever is written there is parsed back by the reader on the other
// side, and a part quoted the wrong way comes out as something else — or takes
// the rest of the line with it. These are the shapes that used to break, kept
// as they are written now.
func TestTheCommandLineQuotesWhatNeedsQuoting(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		program   string
		arguments []string
		want      string
	}{
		{
			name:    "nothing needs quoting",
			program: `C:\trans\trans-window.exe`,
			want:    `C:\trans\trans-window.exe`,
		},
		{
			name:    "a program under Program Files",
			program: `C:\Program Files\trans\trans-window.exe`,
			want:    `"C:\Program Files\trans\trans-window.exe"`,
		},
		{
			name:      "an argument with a space in it",
			program:   `C:\trans\trans-window.exe`,
			arguments: []string{"--target", "My Window"},
			want:      `C:\trans\trans-window.exe --target "My Window"`,
		},
		{
			// The backslash at the end has to be doubled: written as it
			// stands, the closing quote is read as an escaped one and the
			// argument carries on to the end of the line.
			name:      "a backslash before the closing quote",
			program:   `C:\trans\trans-window.exe`,
			arguments: []string{`D:\dir with space\`},
			want:      `C:\trans\trans-window.exe "D:\dir with space\\"`,
		},
		{
			name:      "a quote inside an argument",
			program:   `C:\trans\trans-window.exe`,
			arguments: []string{`say "hi"`},
			want:      `C:\trans\trans-window.exe "say \"hi\""`,
		},
		{
			// An argument with nothing in it has to be written as an empty
			// string, or it is not an argument at all when the line is read
			// back.
			name:      "an argument with nothing in it",
			program:   `C:\trans\trans-window.exe`,
			arguments: []string{""},
			want:      `C:\trans\trans-window.exe ""`,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := commandLineFor(test.program, test.arguments); got != test.want {
				t.Errorf("commandLineFor wrote %q, want %q", got, test.want)
			}
		})
	}
}
