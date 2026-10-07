package requestmodel

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"strings"

	"github.com/tidwall/gjson"
)

// ErrAmbiguousModel is deliberately independent of client-provided values.
var ErrAmbiguousModel = errors.New("ambiguous model fields: use one canonical model field per object")

// ValidateJSONSelectors rejects selectors that first-member, last-member and
// case-insensitive decoders can interpret differently. Inspect only protocol
// selectors, never model properties inside user content or tool schemas.
// Syntax validation remains with the protocol handler, including its existing
// lenient JSON handling. Even null or identical repeated selectors are rejected.
func ValidateJSONSelectors(body []byte) error {
	// Match the existing lenient HTTP reader without rewriting client bytes.
	body = bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})
	return validateSelectorObject(gjson.ParseBytes(body), true)
}

func validateSelectorObject(object gjson.Result, root bool) error {
	if !object.IsObject() {
		return nil
	}
	var seenModel, seenSession, seenType bool
	var invalid bool
	object.ForEach(func(key, value gjson.Result) bool {
		switch {
		case strings.EqualFold(key.Str, "model"):
			invalid = seenModel || key.Str != "model"
			seenModel = true
		case root && strings.EqualFold(key.Str, "session"):
			invalid = seenSession || key.Str != "session" || validateSelectorObject(value, false) != nil
			seenSession = true
		case root && strings.EqualFold(key.Str, "type"):
			// A WS event must not change kind between admission and forwarding.
			invalid = seenType || key.Str != "type"
			seenType = true
		}
		return !invalid
	})
	if invalid {
		return ErrAmbiguousModel
	}
	return nil
}

// ValidateBodySelectors also covers repeated multipart model/session fields.
// File parts are streamed past without allocating a second copy of their data.
func ValidateBodySelectors(contentType string, body []byte) error {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return ValidateJSONSelectors(body)
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var seenModel, seenSession bool
	for {
		part, err := reader.NextPart()
		if err != nil {
			return nil // The endpoint owns malformed multipart errors.
		}
		name := part.FormName()
		switch {
		case strings.EqualFold(name, "model"):
			if seenModel || name != "model" {
				return ErrAmbiguousModel
			}
			seenModel = true
		case strings.EqualFold(name, "session"):
			if seenSession || name != "session" {
				return ErrAmbiguousModel
			}
			seenSession = true
			value, err := io.ReadAll(part)
			if err != nil {
				return nil
			}
			if err := validateSelectorObject(gjson.ParseBytes(value), false); err != nil {
				return err
			}
		}
	}
}
