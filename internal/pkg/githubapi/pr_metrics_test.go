package githubapi

import (
	"testing"
	"time"

	"github.com/google/go-github/v62/github"
)

func TestIsPrStalePending(t *testing.T) {
	t.Parallel()
	timeToDefineStale := 15 * time.Minute

	currentTime := time.Now()
	tests := map[string]struct {
		input  github.CombinedStatus
		result bool
	}{
		"All Success": {
			input: github.CombinedStatus{
				Statuses: []*github.RepoStatus{
					{
						State:   github.String("success"),
						Context: github.String("telefonistka"),
						UpdatedAt: &github.Timestamp{
							Time: currentTime.Add(-10 * time.Minute),
						},
					},
					{
						State:   github.String("success"),
						Context: github.String("circleci"),
						UpdatedAt: &github.Timestamp{
							Time: currentTime.Add(-10 * time.Minute),
						},
					},
					{
						State:   github.String("success"),
						Context: github.String("foobar"),
						UpdatedAt: &github.Timestamp{
							Time: currentTime.Add(-10 * time.Minute),
						},
					},
				},
			},
			result: false,
		},
		"Pending but not stale": {
			input: github.CombinedStatus{
				Statuses: []*github.RepoStatus{
					{
						State:   github.String("pending"),
						Context: github.String("telefonistka"),
						UpdatedAt: &github.Timestamp{
							Time: currentTime.Add(-1 * time.Minute),
						},
					},
				},
			},
			result: false,
		},

		"Pending and stale": {
			input: github.CombinedStatus{
				Statuses: []*github.RepoStatus{
					{
						State:   github.String("pending"),
						Context: github.String("telefonistka"),
						UpdatedAt: &github.Timestamp{
							Time: currentTime.Add(-20 * time.Minute),
						},
					},
				},
			},
			result: true,
		},
	}

	for name, tc := range tests {
		name := name
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result := isPrStalePending(&tc.input, timeToDefineStale)
			if result != tc.result {
				t.Errorf("(%s)Expected %v, got %v", name, tc.result, result)
			}
		})
	}
}
