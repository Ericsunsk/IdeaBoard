package worker

import (
	"context"
	"reflect"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/tasks"
)

type recordingProviderFinalizer struct {
	name       string
	stored     bool
	resultRefs []string
	calls      *[]string
}

func (f recordingProviderFinalizer) finalize(
	_ context.Context,
	_ int64,
	_ string,
	imageRefs []string,
	_ time.Duration,
) ([]string, []tasks.TaskOutput, bool, error) {
	*f.calls = append(*f.calls, f.name+":"+joinFinalizerRefs(imageRefs))
	if !f.stored {
		return imageRefs, nil, false, nil
	}
	return f.resultRefs, []tasks.TaskOutput{{TaskID: f.name, ResultIndex: 0}}, true, nil
}

func joinFinalizerRefs(refs []string) string {
	if len(refs) == 0 {
		return "-"
	}
	result := refs[0]
	for _, ref := range refs[1:] {
		result += "," + ref
	}
	return result
}

func TestProviderFinalizationPipelineChainsFeatureFinalizersAndSkipsRemoteMaterialization(t *testing.T) {
	calls := make([]string, 0, 3)
	pipeline := providerFinalizationPipeline{
		featureFinalizers: []providerFinalizer{
			recordingProviderFinalizer{name: "local-redraw", stored: true, resultRefs: []string{"redraw-result"}, calls: &calls},
			recordingProviderFinalizer{name: "image-extend", stored: true, resultRefs: []string{"extend-result"}, calls: &calls},
		},
		remoteFinalizer: recordingProviderFinalizer{name: "remote", stored: true, resultRefs: []string{"remote-result"}, calls: &calls},
	}

	urls, outputs, err := pipeline.finalize(context.Background(), 42, "task-1", []string{"provider-result"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(urls, []string{"extend-result"}) {
		t.Fatalf("urls=%v, want final feature result", urls)
	}
	if len(outputs) != 1 || outputs[0].TaskID != "image-extend" {
		t.Fatalf("outputs=%#v, want image-extend output", outputs)
	}
	if !reflect.DeepEqual(calls, []string{
		"local-redraw:provider-result",
		"image-extend:redraw-result",
	}) {
		t.Fatalf("calls=%v, want ordered feature finalizers without remote materialization", calls)
	}
}

func TestProviderFinalizationPipelineUsesRemoteMaterializerForPlainProviderResults(t *testing.T) {
	calls := make([]string, 0, 2)
	pipeline := providerFinalizationPipeline{
		featureFinalizers: []providerFinalizer{
			recordingProviderFinalizer{name: "inactive-feature", stored: false, calls: &calls},
		},
		remoteFinalizer: recordingProviderFinalizer{name: "remote", stored: true, resultRefs: []string{"stored-result"}, calls: &calls},
	}

	urls, outputs, err := pipeline.finalize(context.Background(), 42, "task-2", []string{"provider-result"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(urls, []string{"stored-result"}) || len(outputs) != 1 {
		t.Fatalf("urls/outputs=%v/%#v, want remote materialized result", urls, outputs)
	}
	if !reflect.DeepEqual(calls, []string{
		"inactive-feature:provider-result",
		"remote:provider-result",
	}) {
		t.Fatalf("calls=%v, want remote fallback after inactive feature", calls)
	}
}
