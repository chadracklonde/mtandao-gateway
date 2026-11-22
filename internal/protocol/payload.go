package protocol

import (
	"errors"
	"fmt"
	"math"
	"regexp"
)

// ErrInvalidPayload is returned when an operation payload does not match its schema.
var ErrInvalidPayload = errors.New("invalid command payload")

var (
	voucherCodePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9 _.-]{1,120}$`)
	// Ticket comment in the Mikhmon / Mtandao Ticket format: vc-DDMMYY-HHMM <note>.
	// The router's profile script only logs a sale for users whose comment starts with vc- or up-.
	ticketCommentPattern = regexp.MustCompile(`^(vc|up)-[0-9]{6}-[0-9]{4}( [A-Za-z0-9 .'-]{1,30})?$`)
)

const (
	maxDurationMinutes = 525600
	maxDataLimitMB     = 10_000_000
)

// VoucherCreate is the validated payload of hotspot.voucher.create.
type VoucherCreate struct {
	Code            string
	Profile         string // optional
	DurationMinutes int    // optional: usage time limit (limit-uptime); 0 means none
	DataLimitMB     int    // optional: data limit (limit-bytes-total); 0 means none
	Comment         string // optional: ticket comment, vc-DDMMYY-HHMM <note>
}

// ParseVoucherCreate validates the payload strictly: unknown fields are refused,
// so nothing unreviewed can ever reach the router.
func ParseVoucherCreate(payload map[string]any) (VoucherCreate, error) {
	var out VoucherCreate
	for key := range payload {
		if key != "code" && key != "profile" && key != "duration_minutes" && key != "data_limit_mb" && key != "comment" {
			return out, fmt.Errorf("%w: unexpected field %q", ErrInvalidPayload, key)
		}
	}
	code, err := requireCode(payload)
	if err != nil {
		return out, err
	}
	out.Code = code
	if raw, ok := payload["profile"]; ok {
		profile, isString := raw.(string)
		if !isString || !profileNamePattern.MatchString(profile) {
			return out, fmt.Errorf("%w: profile", ErrInvalidPayload)
		}
		out.Profile = profile
	}
	if raw, ok := payload["duration_minutes"]; ok {
		number, isNumber := raw.(float64)
		if !isNumber || number != math.Trunc(number) || number < 1 || number > maxDurationMinutes {
			return out, fmt.Errorf("%w: duration_minutes", ErrInvalidPayload)
		}
		out.DurationMinutes = int(number)
	}
	if raw, ok := payload["data_limit_mb"]; ok {
		number, isNumber := raw.(float64)
		if !isNumber || number != math.Trunc(number) || number < 1 || number > maxDataLimitMB {
			return out, fmt.Errorf("%w: data_limit_mb", ErrInvalidPayload)
		}
		out.DataLimitMB = int(number)
	}
	if raw, ok := payload["comment"]; ok {
		comment, isString := raw.(string)
		if !isString || !ticketCommentPattern.MatchString(comment) {
			return out, fmt.Errorf("%w: comment", ErrInvalidPayload)
		}
		out.Comment = comment
	}
	return out, nil
}

// ParseVoucherDisable validates the payload of hotspot.voucher.disable.
func ParseVoucherDisable(payload map[string]any) (string, error) {
	for key := range payload {
		if key != "code" {
			return "", fmt.Errorf("%w: unexpected field %q", ErrInvalidPayload, key)
		}
	}
	return requireCode(payload)
}

// RequireEmptyPayload validates operations that take no arguments.
func RequireEmptyPayload(payload map[string]any) error {
	if len(payload) != 0 {
		return fmt.Errorf("%w: this operation takes no payload", ErrInvalidPayload)
	}
	return nil
}

// IsWriteOperation reports whether an operation changes router state. Write
// operations are never re-executed after an interruption.
func IsWriteOperation(operation string) bool {
	return operation == "hotspot.voucher.create" || operation == "hotspot.voucher.disable"
}

func requireCode(payload map[string]any) (string, error) {
	code, ok := payload["code"].(string)
	if !ok || !voucherCodePattern.MatchString(code) {
		return "", fmt.Errorf("%w: code", ErrInvalidPayload)
	}
	return code, nil
}
