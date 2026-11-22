package routeros

import (
	"errors"
	"regexp"
	"strings"
)

// ErrUnsupportedRequest is returned for any RouterOS request outside the allow-list.
var ErrUnsupportedRequest = errors.New("unsupported RouterOS request")

type hotspotRule struct {
	attrs    map[string]bool // =key=value
	queries  map[string]bool // ?key=value
	proplist map[string]bool // =.proplist=a,b
	required []string        // attributes that must be present
}

func set(keys ...string) map[string]bool {
	out := make(map[string]bool, len(keys))
	for _, key := range keys {
		out[key] = true
	}
	return out
}

// hotspotRules is the complete list of RouterOS commands the Gateway may issue
// for Hotspot operations, with the exact arguments each accepts. Anything else,
// including any other command or attribute, is refused before reaching the router.
var hotspotRules = map[string]hotspotRule{
	"/ip/hotspot/user/profile/print": {
		proplist: set("name", "rate-limit", "session-timeout", "idle-timeout", "shared-users"),
	},
	"/ip/hotspot/active/print": {
		queries:  set("user"),
		proplist: set(".id", "user", "address", "mac-address", "uptime", "session-time-left", "bytes-in", "bytes-out"),
	},
	"/ip/hotspot/user/print": {
		queries:  set("name"),
		proplist: set(".id", "name", "disabled", "profile"),
	},
	"/ip/hotspot/user/add": {
		attrs:    set("name", "password", "profile", "limit-uptime", "limit-bytes-total", "comment"),
		required: []string{"name", "password"},
	},
	"/ip/hotspot/user/set": {
		attrs:    set(".id", "disabled"),
		required: []string{".id", "disabled"},
	},
	"/ip/hotspot/active/remove": {
		attrs:    set(".id"),
		required: []string{".id"},
	},
}

var (
	routerIDPattern  = regexp.MustCompile(`^\*[0-9A-Fa-f]{1,16}$`)
	safeValuePattern = regexp.MustCompile(`^[A-Za-z0-9 _.\-]{1,120}$`)
	uptimePattern    = regexp.MustCompile(`^([0-9]{1,4}d)?([0-9]{1,2}h)?([0-9]{1,2}m)?$`)
	bytesPattern     = regexp.MustCompile(`^[1-9][0-9]{0,12}$`)
	commentPattern   = regexp.MustCompile(`^(vc|up)-[0-9]{6}-[0-9]{4}( [A-Za-z0-9 .'-]{1,30})?$`)
)

// ValidateHotspotWords checks one RouterOS API sentence against the allow-list.
func ValidateHotspotWords(words []string) error {
	if len(words) == 0 {
		return ErrUnsupportedRequest
	}
	rule, ok := hotspotRules[words[0]]
	if !ok {
		return ErrUnsupportedRequest
	}
	seen := map[string]bool{}
	for _, word := range words[1:] {
		if len(word) < 3 || len(word) > 300 || strings.ContainsAny(word, "\x00\r\n") {
			return ErrUnsupportedRequest
		}
		kind, rest := word[0], word[1:]
		key, value, found := strings.Cut(rest, "=")
		if !found || key == "" || seen[kind2s(kind)+key] {
			return ErrUnsupportedRequest
		}
		seen[kind2s(kind)+key] = true
		switch {
		case kind == '=' && key == ".proplist":
			if len(rule.proplist) == 0 || !validProplist(rule.proplist, value) {
				return ErrUnsupportedRequest
			}
		case kind == '=' && rule.attrs[key]:
			if !validAttribute(key, value) {
				return ErrUnsupportedRequest
			}
		case kind == '?' && rule.queries[key]:
			if !safeValuePattern.MatchString(value) {
				return ErrUnsupportedRequest
			}
		default:
			return ErrUnsupportedRequest
		}
	}
	for _, key := range rule.required {
		if !seen["="+key] {
			return ErrUnsupportedRequest
		}
	}
	return nil
}

func kind2s(kind byte) string { return string(kind) }

func validProplist(allowed map[string]bool, value string) bool {
	fields := strings.Split(value, ",")
	if len(fields) == 0 || len(fields) > 10 {
		return false
	}
	dup := map[string]bool{}
	for _, field := range fields {
		if !allowed[field] || dup[field] {
			return false
		}
		dup[field] = true
	}
	return true
}

func validAttribute(key, value string) bool {
	switch key {
	case ".id":
		return routerIDPattern.MatchString(value)
	case "disabled":
		return value == "yes" || value == "no"
	case "limit-uptime":
		return value != "" && uptimePattern.MatchString(value)
	case "limit-bytes-total":
		return bytesPattern.MatchString(value)
	case "comment":
		return commentPattern.MatchString(value)
	default: // name, password, profile
		return safeValuePattern.MatchString(value)
	}
}
