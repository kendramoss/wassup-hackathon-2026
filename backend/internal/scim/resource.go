package scim

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Schema URNs (RFC 7643, RFC 7644).
const (
	SchemaUser     = "urn:ietf:params:scim:schemas:core:2.0:User"
	SchemaList     = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	SchemaError    = "urn:ietf:params:scim:api:messages:2.0:Error"
	SchemaPatch    = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	SchemaSPConfig = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	SchemaResType  = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	SchemaSchema   = "urn:ietf:params:scim:schemas:core:2.0:Schema"
	ContentType    = "application/scim+json"
	MaxPage        = 100
)

// UserID is the stable SCIM id of a member: opaque, and different in every
// workspace, so it reveals neither the email nor another workspace.
func UserID(workspace, email string) string {
	h := sha256.Sum256([]byte("scim-user:" + workspace + ":" + email))
	return "u" + hex.EncodeToString(h[:12])
}

// Value is one entry of a multi-valued attribute.
type Value struct {
	Value   string `json:"value"`
	Primary bool   `json:"primary,omitempty"`
	Type    string `json:"type,omitempty"`
}

// Meta is the resource metadata.
type Meta struct {
	ResourceType string     `json:"resourceType"`
	Location     string     `json:"location,omitempty"`
	Created      *time.Time `json:"created,omitempty"`
}

// User is the User resource this server returns.
type User struct {
	Schemas  []string `json:"schemas"`
	ID       string   `json:"id"`
	UserName string   `json:"userName"`
	Active   bool     `json:"active"`
	Emails   []Value  `json:"emails"`
	Roles    []Value  `json:"roles"`
	Meta     Meta     `json:"meta"`
}

// NewUser renders a member.
func NewUser(workspace, email, role, location string) User {
	id := UserID(workspace, email)
	return User{Schemas: []string{SchemaUser}, ID: id, UserName: email, Active: true,
		Emails: []Value{{Value: email, Primary: true, Type: "work"}}, Roles: []Value{{Value: role, Primary: true}},
		Meta: Meta{ResourceType: "User", Location: location + "/" + id}}
}

// UserInput is what a POST or PUT carries; unknown attributes are ignored
// (RFC 7644 3.3: the server may ignore attributes it does not support).
type UserInput struct {
	UserName string          `json:"userName"`
	Active   *bool           `json:"active"`
	Emails   []Value         `json:"emails"`
	Roles    json.RawMessage `json:"roles"`
}

// Email returns the normalised email of the input: userName, else the
// primary email.
func (in UserInput) Email() string {
	e := in.UserName
	if !strings.Contains(e, "@") {
		for _, v := range in.Emails {
			if v.Primary || e == "" || !strings.Contains(e, "@") {
				e = v.Value
			}
		}
	}
	return NormalEmail(e)
}

var emailPattern = regexp.MustCompile(`^[a-z0-9._%+'-]{1,64}@[a-z0-9-]+(\.[a-z0-9-]+)+$`)

// NormalEmail lowercases and trims e, or returns "" when it is not an email.
func NormalEmail(e string) string {
	e = strings.ToLower(strings.TrimSpace(e))
	if len(e) > 254 || !emailPattern.MatchString(e) {
		return ""
	}
	return e
}

// ErrRole is a roles value that is not viewer, editor or admin.
var ErrRole = errors.New("roles must be one of viewer, editor, admin")

// ParseRoles reads a roles attribute: an array of {value} or of strings, or
// one string. "" means none given.
func ParseRoles(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return checkRole(one)
	}
	var vals []json.RawMessage
	if err := json.Unmarshal(raw, &vals); err != nil {
		return "", ErrRole
	}
	if len(vals) == 0 {
		return "", nil
	}
	if len(vals) > 1 {
		return "", errors.New("give one role")
	}
	var v Value
	if json.Unmarshal(vals[0], &v) == nil && v.Value != "" {
		return checkRole(v.Value)
	}
	if json.Unmarshal(vals[0], &one) == nil {
		return checkRole(one)
	}
	return "", ErrRole
}

func checkRole(s string) (string, error) {
	switch s = strings.ToLower(strings.TrimSpace(s)); s {
	case "viewer", "editor", "admin":
		return s, nil
	}
	return "", ErrRole
}

// ErrFilter is a filter this server does not support.
var ErrFilter = errors.New(`only filter=userName eq "email" is supported`)

var filterPattern = regexp.MustCompile(`(?i)^\s*(username|emails(?:\.value)?)\s+eq\s+"([^"\\]{1,254})"\s*$`)

// ParseFilter reads the one filter identity providers send to look a user
// up. "" matches everyone.
func ParseFilter(f string) (email string, all bool, err error) {
	if strings.TrimSpace(f) == "" {
		return "", true, nil
	}
	m := filterPattern.FindStringSubmatch(f)
	if m == nil {
		return "", false, ErrFilter
	}
	return strings.ToLower(strings.TrimSpace(m[2])), false, nil
}

// Page reads startIndex and count (1-based, RFC 7644 3.4.2.4).
func Page(start, count string) (int, int, error) {
	s, c := 1, MaxPage
	if start != "" {
		n, err := strconv.Atoi(start)
		if err != nil {
			return 0, 0, fmt.Errorf("startIndex must be a number")
		}
		s = max(1, n)
	}
	if count != "" {
		n, err := strconv.Atoi(count)
		if err != nil {
			return 0, 0, fmt.Errorf("count must be a number")
		}
		c = min(max(0, n), MaxPage)
	}
	return s, c, nil
}

// PatchOp is one operation of a PATCH request.
type PatchOp struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

// PatchRequest is the body of a PATCH.
type PatchRequest struct {
	Schemas    []string  `json:"schemas"`
	Operations []PatchOp `json:"Operations"`
}

// Change is what a PATCH asks for: nil fields are unchanged.
type Change struct {
	Active *bool
	Role   string
}

// ErrPatch is a PATCH this server does not support.
var ErrPatch = errors.New("only active and roles can be changed")

// ParsePatch reads the operations identity providers send: replace or add on
// active or roles, by path or as a value object; remove of roles is ignored.
// Entra ID sends booleans as strings ("False"), which is accepted.
func ParsePatch(req PatchRequest) (Change, error) {
	var c Change
	if len(req.Operations) == 0 || len(req.Operations) > 10 {
		return c, ErrPatch
	}
	for _, op := range req.Operations {
		kind := strings.ToLower(op.Op)
		path := strings.ToLower(strings.TrimSpace(op.Path))
		if kind == "remove" {
			if strings.HasPrefix(path, "roles") {
				continue
			}
			return c, ErrPatch
		}
		if kind != "replace" && kind != "add" {
			return c, ErrPatch
		}
		switch path {
		case "active":
			b, err := parseBool(op.Value)
			if err != nil {
				return c, err
			}
			c.Active = &b
		case "roles":
			r, err := ParseRoles(op.Value)
			if err != nil {
				return c, err
			}
			c.Role = r
		case "":
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(op.Value, &obj); err != nil {
				return c, ErrPatch
			}
			for k, v := range obj {
				switch strings.ToLower(k) {
				case "active":
					b, err := parseBool(v)
					if err != nil {
						return c, err
					}
					c.Active = &b
				case "roles":
					r, err := ParseRoles(v)
					if err != nil {
						return c, err
					}
					c.Role = r
				}
			}
		default:
			return c, ErrPatch
		}
	}
	return c, nil
}

func parseBool(raw json.RawMessage) (bool, error) {
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b, nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	}
	return false, errors.New("active must be true or false")
}
