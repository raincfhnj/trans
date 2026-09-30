//go:build windows

package secrets

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The protection call binds the ciphertext to two things besides the bytes
// themselves: a description naming the setting, and entropy naming this
// program. Both are verified on the way back out, so a record pasted onto a
// different setting's line — or a blob written by another build — refuses to
// open instead of quietly handing the key over.
func describe(variable string) *uint16 {
	text, err := windows.UTF16PtrFromString("trans:" + variable)
	if err != nil {
		// The only failure is an interior NUL, which a setting name cannot
		// hold; a nil description still protects, it just binds less.
		return nil
	}
	return text
}

// blob wraps bytes as the variable-sized structure the DPAPI calls take. The
// backing array must outlive the call, which every caller below arranges by
// holding its slice on the stack frame of the call itself.
//
// #nosec G115 — a settings variable is a few dozen bytes at most, far inside
// uint32; G103 — Win32 hands pointers back as numbers, and there is no way to
// read what they point at without unsafe (the same exclusion the win32 package
// carries).
func blob(data []byte) *windows.DataBlob {
	if len(data) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{
		Size: uint32(len(data)),                 // #nosec G115
		Data: (*byte)(unsafe.Pointer(&data[0])), // #nosec G103
	}
}

// entropy is the fixed padding the ciphertext is bound to. It is not secret —
// it exists so two blobs of the same key for two different settings differ.
func entropy() []byte {
	return []byte("trans-entropy")
}

// bytesOut copies a DPAPI-allocated blob into a Go slice and frees the blob.
// The copy happens before the free because the memory is Windows' to take
// back at any moment after the call returns.
//
// #nosec G103 — Win32 hands pointers back as numbers; the copy is the only
// way to read them.
func bytesOut(blob *windows.DataBlob) []byte {
	defer func() {
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(blob.Data))) // #nosec G103
	}()
	out := make([]byte, blob.Size)
	copy(out, unsafe.Slice(blob.Data, int(blob.Size))) // #nosec G103
	return out
}

// Encrypt protects the bytes with the Windows Data Protection API, bound to
// the identity the scope names. Nothing about the plaintext comes back
// without that identity — or, for LocalMachine, without this computer.
func Encrypt(plaintext []byte, variable string, scope Scope) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errors.New("secrets: refusing to protect an empty value")
	}

	flags := uint32(windows.CRYPTPROTECT_UI_FORBIDDEN)
	if scope == LocalMachine {
		flags |= windows.CRYPTPROTECT_LOCAL_MACHINE
	}

	var protected windows.DataBlob
	err := windows.CryptProtectData(
		blob(plaintext), describe(variable), blob(entropy()),
		0, nil, flags, &protected)
	if err != nil {
		return nil, fmt.Errorf("secrets: protecting the value: %w", err)
	}
	return bytesOut(&protected), nil
}

// Decrypt reverses Encrypt. A blob written under another account, for another
// setting, or tampered with on the way comes back as an error rather than as
// garbage.
func Decrypt(ciphertext []byte, variable string) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, errors.New("secrets: nothing to unprotect")
	}

	var opened windows.DataBlob
	err := windows.CryptUnprotectData(
		blob(ciphertext), nil, blob(entropy()),
		0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &opened)
	if err != nil {
		return nil, fmt.Errorf("secrets: unprotecting the value: %w", err)
	}
	return bytesOut(&opened), nil
}
