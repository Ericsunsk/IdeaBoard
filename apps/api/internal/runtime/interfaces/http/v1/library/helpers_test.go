package library

import (
	"net/http"
	"testing"

	"ideaboard/api/internal/runtime/interfaces/http/v1/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type v1Call struct {
	method  string
	path    string
	body    string
	userID  int64
	headers map[string]string
}

type staticUserDirectory struct {
	*testsupport.StaticUserDirectory
}

func (d *staticUserDirectory) ensure(userID int64) uuid.UUID {
	return d.Ensure(userID)
}

func newStaticUserDirectory() *staticUserDirectory {
	return &staticUserDirectory{StaticUserDirectory: testsupport.NewStaticUserDirectory()}
}

func doV1(t *testing.T, engine *gin.Engine, call v1Call) (int, http.Header, map[string]any) {
	t.Helper()
	return testsupport.Do(t, engine, testsupport.Call{
		Method:  call.method,
		Path:    call.path,
		Body:    call.body,
		UserID:  call.userID,
		Headers: call.headers,
	})
}

func doV1RoundTrip(t *testing.T, engine *gin.Engine, call v1Call, reqIDPrefix string) (int, http.Header, string) {
	t.Helper()
	return testsupport.DoRoundTrip(t, engine, testsupport.Call{
		Method:  call.method,
		Path:    call.path,
		Body:    call.body,
		UserID:  call.userID,
		Headers: call.headers,
	}, reqIDPrefix)
}

var v1Data = testsupport.Data
var v1ErrorCode = testsupport.ErrorCode
var v1GapsDeclare = testsupport.GapsDeclare
var v1GapsStage = testsupport.GapsStage
var v1GapsChecksum = testsupport.GapsChecksum
var v1MediaAssetID = testsupport.MediaAssetID
