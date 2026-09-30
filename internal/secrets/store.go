package secrets

import (
	"errors"
)

// Store protects value and files it under variable in the configuration
// directory, replacing any record for that variable. The scope decides whose
// identity the ciphertext is bound to — see the Scope constants.
func Store(directory, variable string, value []byte, scope Scope) error {
	if variable == "" {
		return errors.New("secrets: no setting named")
	}
	protected, err := Encrypt(value, variable, scope)
	if err != nil {
		return err
	}
	contents, err := read(directory)
	if err != nil {
		return err
	}
	if contents == nil {
		contents = &File{}
	}
	kept := contents.Records[:0]
	for _, record := range contents.Records {
		if record.Variable != variable {
			kept = append(kept, record)
		}
	}
	kept = append(kept, Record{Variable: variable, Protected: protected})
	contents.Records = kept
	return write(directory, contents)
}

// Load reads the variable back and decrypts it. ErrNotFound answers when the
// store has no record for it; on other systems ErrUnsupported answers first
// from Decrypt, after the record has been found.
func Load(directory, variable string) ([]byte, error) {
	if variable == "" {
		return nil, errors.New("secrets: no setting named")
	}
	contents, err := read(directory)
	if err != nil {
		return nil, err
	}
	if contents == nil {
		return nil, ErrNotFound
	}
	for _, record := range contents.Records {
		if record.Variable != variable {
			continue
		}
		return Decrypt(record.Protected, variable)
	}
	return nil, ErrNotFound
}

// Delete drops the variable's record, leaving every other one as it stands.
// A record that is not there is not an error: the store is already in the
// state the caller wanted.
func Delete(directory, variable string) error {
	contents, err := read(directory)
	if err != nil {
		return err
	}
	if contents == nil {
		return nil
	}
	kept := contents.Records[:0]
	for _, record := range contents.Records {
		if record.Variable != variable {
			kept = append(kept, record)
		}
	}
	if len(kept) == len(contents.Records) {
		return nil
	}
	contents.Records = kept
	return write(directory, contents)
}
