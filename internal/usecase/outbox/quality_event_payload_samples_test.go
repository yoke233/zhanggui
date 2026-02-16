package outbox

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInferQualityFieldsFromPayloadSampleContracts(t *testing.T) {
	testCases := []struct {
		name         string
		source       string
		sampleFile   string
		wantCategory string
		wantResult   string
	}{
		{
			name:         "github review sample infers review changes requested",
			source:       "github",
			sampleFile:   "github-review.sample.json",
			wantCategory: qualityCategoryReview,
			wantResult:   qualityResultChangesRequested,
		},
		{
			name:         "github check run sample infers ci fail",
			source:       "github",
			sampleFile:   "github-check-run.sample.json",
			wantCategory: qualityCategoryCI,
			wantResult:   qualityResultFail,
		},
		{
			name:         "gitlab pipeline sample infers ci fail",
			source:       "gitlab",
			sampleFile:   "gitlab-pipeline.sample.json",
			wantCategory: qualityCategoryCI,
			wantResult:   qualityResultFail,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			payload := mustReadPayloadSample(t, tc.sampleFile)

			fields, err := inferQualityFieldsFromPayload(tc.source, payload)
			if err != nil {
				t.Fatalf("inferQualityFieldsFromPayload(%q, %q) error = %v", tc.source, tc.sampleFile, err)
			}

			if fields.Category != tc.wantCategory {
				t.Fatalf("fields.Category = %q, want %q", fields.Category, tc.wantCategory)
			}
			if fields.Result != tc.wantResult {
				t.Fatalf("fields.Result = %q, want %q", fields.Result, tc.wantResult)
			}
		})
	}
}

func mustReadPayloadSample(t *testing.T, sampleFile string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) = false")
	}

	samplePath := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "mailbox", sampleFile)
	content, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", samplePath, err)
	}

	return string(content)
}
