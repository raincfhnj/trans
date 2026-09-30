package secrets

import "encoding/json"

// marshal keeps the document stable between writes: records keep insertion
// order and the file ends with a newline, so a diff of two saves differs only
// where a record actually changed.
func marshal(contents *File) ([]byte, error) {
	encoded, err := json.MarshalIndent(contents, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// unmarshal answers the store as it stands, refusing anything that is not one
// so a corrupted file is reported rather than silently emptied.
func unmarshal(encoded []byte) (*File, error) {
	contents := &File{}
	if err := json.Unmarshal(encoded, contents); err != nil {
		return nil, err
	}
	return contents, nil
}
