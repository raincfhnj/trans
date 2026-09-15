// Package win32 is the little of Windows the panel needs: the window it opens
// over, the keyboard it types with, the clipboard it hands the prompt to, and
// the console it draws in. Everything here is compiled only on Windows — the
// herdr plugin needs none of it — except the parts that are pure text, which are
// kept in match.go so they can be tested anywhere.
package win32
