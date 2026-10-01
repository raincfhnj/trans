package win32

// ClipboardSnapshot is the clipboard kept to one side so that it can be put
// back. A prompt borrows the clipboard for the moment a pane needs to take it,
// and whoever put a screenshot or a set of files there did not agree to lose
// it: what can be copied back is, and what cannot is left alone rather than
// written over.
//
// This is the whole of what the delivery path needs to know about a snapshot,
// and it is written without a build tag so that the delivery policy — and the
// tests for it — can be built and run where Win32 is not, which is how the
// policy is checked at all. The implementation behind it lives in
// clipboard_windows.go.
type ClipboardSnapshot interface {
	// Restore puts the clipboard back as the snapshot found it. It answers an
	// error when the clipboard could not be opened or written, which is worth
	// saying: the pane has the prompt on the clipboard and the author's own
	// content is still missing.
	Restore() error
	// Holds says whether there is anything to put back. A snapshot taken of an
	// empty clipboard holds nothing, and the clipboard is then left as the
	// delivery left it rather than emptied a second time.
	Holds() bool
}
