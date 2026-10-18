package testsupport

import (
	"net/http"
	"strconv"
	"strings"
)

// Test principal injector headers (P2 literal convergence).
//
// HTTP-layer tests inject a principal through test-only headers instead of a
// real Auth session: HeaderTestUser carries the numeric user id, HeaderTestRole
// the identity role (absent means anonymous), HeaderTestCaps an optional
// comma-separated capability set for control-plane engines. These spellings
// are the single home: test engines must read them via these constants and
// test callers must set them via the Set* helpers below, never hand-written
// "X-Test-*" literals.
const (
	HeaderTestUser = "X-Test-User"
	HeaderTestRole = "X-Test-Role"
	HeaderTestCaps = "X-Test-Caps"
)

// SetTestUser injects the numeric test user id on an outgoing test request.
func SetTestUser(req *http.Request, userID int64) {
	req.Header.Set(HeaderTestUser, strconv.FormatInt(userID, 10))
}

// SetTestRole injects the identity role on an outgoing test request.
// Empty roles are left unset so the engine default applies.
func SetTestRole(req *http.Request, role string) {
	if strings.TrimSpace(role) == "" {
		return
	}
	req.Header.Set(HeaderTestRole, role)
}

// SetTestCaps injects the comma-separated capability set on an outgoing
// test request. Empty sets are left unset.
func SetTestCaps(req *http.Request, caps []string) {
	if len(caps) == 0 {
		return
	}
	req.Header.Set(HeaderTestCaps, strings.Join(caps, ","))
}
