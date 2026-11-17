// This testdata command is run by apps/ai-proxy's explicit test:contract.
// Ordinary Go tests do not need Node, and all Edge fetches are mocked.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"reflect"

	"ideaboard/api/internal/platform/workerwire"
)

type requests struct {
	Full      workerwire.GenerateRequest `json:"full"`
	Minimal   workerwire.GenerateRequest `json:"minimal"`
	Inherited workerwire.GenerateRequest `json:"inherited"`
	Probe     workerwire.DryRunRequest   `json:"probe"`
}

type response struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

func check(ok bool, message string) {
	if !ok {
		panic(message)
	}
}

// Round trips compare values, not duplicated key arrays. Unknown fields,
// absent vs zero, and silently dropped required fields all fail here.
func roundTrip(raw []byte, value any) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		panic(err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var before, after any
	if err := json.Unmarshal(raw, &before); err != nil {
		panic(err)
	}
	if err := json.Unmarshal(encoded, &after); err != nil {
		panic(err)
	}
	check(reflect.DeepEqual(before, after), fmt.Sprintf("wire fields lost or added:\n%s\n%s", raw, encoded))
}

func main() {
	check(len(os.Args) >= 2, "expected encode or verify")
	switch os.Args[1] {
	case "encode":
		check(len(os.Args) == 3, "expected request fixture path")
		raw, err := os.ReadFile(os.Args[2])
		if err != nil {
			panic(err)
		}
		var fixture requests
		roundTrip(raw, &fixture)
		var output bytes.Buffer
		if err := jpeg.Encode(&output, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
			panic(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(struct {
			Requests requests `json:"requests"`
			JPEG     string   `json:"jpeg"`
		}{fixture, base64.StdEncoding.EncodeToString(output.Bytes())}); err != nil {
			panic(err)
		}
	case "verify":
		var responses map[string]response
		if err := json.NewDecoder(os.Stdin).Decode(&responses); err != nil {
			panic(err)
		}
		check(len(responses) == 11, "expected three successes, one dry-run and seven failure codes")
		var inherited workerwire.Success
		roundTrip(responses["inherited"].Body, &inherited)
		check(responses["inherited"].Status == 200 && inherited.TaskID == "wire-inherited" && inherited.Provider == "gwllm", "input-aspect generation did not complete")
		check(inherited.ImageURL == "https://media.test/output/inherited", "input-aspect output URL changed")
		for _, name := range []string{"full", "minimal"} {
			result := responses[name]
			check(result.Status == 200, name+" generation did not complete")
			var success workerwire.Success
			roundTrip(result.Body, &success)
			check(success.TaskID == "wire-"+name && success.Status == "completed", "wrong generation identity/status")
			check(success.Provider == "workers-ai" && success.Model == "@cf/contract-image", "wrong generation provider/model")
			check(success.ContentType == "image/jpeg" && success.DurationMS >= 0, "wrong MIME/duration")
			check(success.Receipt.UpstreamStatus == 200 && success.Receipt.Provider == success.Provider && success.Receipt.Model == success.Model, "receipt facts lost")
			if name == "full" {
				check(success.Seed != nil && *success.Seed == 0, "explicit upstream seed zero lost")
				check(success.ImageURL == "https://media.test/public/full", "public_url not honored")
				check(success.Receipt.LogID != nil && *success.Receipt.LogID == "contract-log" && success.Receipt.CacheStatus != nil && *success.Receipt.CacheStatus == "MISS", "optional receipt fields lost")
			} else {
				check(success.Seed == nil && success.Receipt.LogID == nil && success.Receipt.CacheStatus == nil, "absent seed/receipt gained a value")
				check(success.ImageURL == "https://media.test/output/minimal", "upload URL query not removed")
			}
		}
		var dry workerwire.DryRunResponse
		roundTrip(responses["probe"].Body, &dry)
		check(responses["probe"].Status == 200 && dry.Provider == "workers-ai" && dry.Model == "@cf/contract-image", "wrong dry-run identity/status")
		check(dry.PathSource != "" && dry.Target.Method == "POST" && dry.Target.URL != "", "dry-run target facts lost")
		failures := map[string]struct {
			code             string
			status, upstream int
		}{
			"bad_request":  {workerwire.ErrorBadRequest, 400, 0},
			"unauthorized": {workerwire.ErrorUnauthorized, 401, 0},
			"upstream":     {workerwire.ErrorUpstream, 503, 503},
			"unsupported":  {workerwire.ErrorUnsupportedModel, 422, 202},
			"input_fetch":  {workerwire.ErrorInputFetchFailed, 400, 0},
			"upload":       {workerwire.ErrorUploadFailed, 502, 403},
			"internal":     {workerwire.ErrorAdapterInternal, 500, 0},
		}
		for name, want := range failures {
			result := responses[name]
			var failure workerwire.Failure
			roundTrip(result.Body, &failure)
			check(result.Status == want.status && failure.Status == "failed" && failure.Error.Code == want.code, "failure code/status mismatch: "+name)
			taskID := "wire-minimal"
			if name == "unauthorized" {
				taskID = "unknown"
			}
			if name == "input_fetch" {
				taskID = "wire-full"
			}
			check(failure.TaskID == taskID && failure.Error.Message != "", "failure identity/message lost: "+name)
			if want.upstream == 0 {
				check(failure.Error.UpstreamStatus == nil, "absent upstream_status gained a value: "+name)
			} else {
				check(failure.Error.UpstreamStatus != nil && *failure.Error.UpstreamStatus == want.upstream, "upstream_status lost: "+name)
			}
		}
		fmt.Println("Go decoded actual Edge success, dry-run and all seven failure envelopes")
	default:
		panic("expected encode or verify")
	}
}
