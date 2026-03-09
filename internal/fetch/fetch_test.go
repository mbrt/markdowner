package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mbrt/markdowner/internal/output"
)

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestOverridesApply(t *testing.T) {
	baseDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	overrideDate := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		doc       output.Doc
		overrides Overrides
		want      output.Frontmatter
	}{
		{
			name: "no overrides leaves frontmatter unchanged",
			doc: output.Doc{Frontmatter: output.Frontmatter{
				Title:  "Original Title",
				Author: "Original Author",
				Date:   &baseDate,
				Tags:   []string{"a"},
			}},
			want: output.Frontmatter{
				Title:  "Original Title",
				Author: "Original Author",
				Date:   &baseDate,
				Tags:   []string{"a"},
			},
		},
		{
			name:      "override title",
			doc:       output.Doc{Frontmatter: output.Frontmatter{Title: "Old", Author: "Auth"}},
			overrides: Overrides{Title: "New Title"},
			want:      output.Frontmatter{Title: "New Title", Author: "Auth"},
		},
		{
			name:      "override author",
			doc:       output.Doc{Frontmatter: output.Frontmatter{Title: "T", Author: "Old"}},
			overrides: Overrides{Author: "New Author"},
			want:      output.Frontmatter{Title: "T", Author: "New Author"},
		},
		{
			name:      "override source",
			doc:       output.Doc{Frontmatter: output.Frontmatter{Title: "T"}},
			overrides: Overrides{Source: "mysite"},
			want:      output.Frontmatter{Title: "T", Source: "mysite"},
		},
		{
			name:      "override date",
			doc:       output.Doc{Frontmatter: output.Frontmatter{Title: "T", Date: &baseDate}},
			overrides: Overrides{Date: &overrideDate},
			want:      output.Frontmatter{Title: "T", Date: &overrideDate},
		},
		{
			name:      "override tags",
			doc:       output.Doc{Frontmatter: output.Frontmatter{Title: "T", Tags: []string{"old"}}},
			overrides: Overrides{Tags: []string{"new", "tags"}},
			want:      output.Frontmatter{Title: "T", Tags: []string{"new", "tags"}},
		},
		{
			name: "override all fields",
			doc:  output.Doc{Frontmatter: output.Frontmatter{Title: "Old", Author: "Old", Date: &baseDate, Tags: []string{"old"}}},
			overrides: Overrides{
				Title:  "New Title",
				Author: "New Author",
				Source: "web",
				Date:   &overrideDate,
				Tags:   []string{"x", "y"},
			},
			want: output.Frontmatter{Title: "New Title", Author: "New Author", Source: "web", Date: &overrideDate, Tags: []string{"x", "y"}},
		},
		{
			name: "empty tags slice does not clear existing tags",
			doc:  output.Doc{Frontmatter: output.Frontmatter{Title: "T", Tags: []string{"keep"}}},
			want: output.Frontmatter{Title: "T", Tags: []string{"keep"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.overrides.apply(&tt.doc)
			assert.Equal(t, tt.want, tt.doc.Frontmatter)
		})
	}
}

const testHTML = `<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body><article><p>Hello world.</p></article></body>
</html>`

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			fmt.Fprint(w, testHTML)
		case "/error":
			http.Error(w, "broken", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testClient() Client {
	return Client{RetryBackoff: time.Nanosecond}
}

func collectResults(ch <-chan output.Result) []output.Result {
	var results []output.Result
	for r := range ch {
		results = append(results, r)
	}
	return results
}

func TestFetchURLs_SingleURL(t *testing.T) {
	srv := testServer(t)
	f := Fetcher{Client: testClient(), Timeout: 5 * time.Second}

	results := collectResults(f.FetchURLs(context.Background(), []string{srv.URL + "/ok"}))
	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	assert.Contains(t, results[0].Doc.Markdown, "Hello world")
	assert.Equal(t, srv.URL+"/ok", results[0].Doc.Frontmatter.URL)
}

func TestFetchURLs_MultipleURLs(t *testing.T) {
	srv := testServer(t)
	f := Fetcher{Client: testClient(), Parallel: 2, Timeout: 5 * time.Second}

	urls := []string{srv.URL + "/ok", srv.URL + "/ok", srv.URL + "/ok"}
	results := collectResults(f.FetchURLs(context.Background(), urls))
	require.Len(t, results, 3)
	for _, r := range results {
		assert.NoError(t, r.Err)
		assert.Contains(t, r.Doc.Markdown, "Hello world")
	}
}

func TestFetchURLs_HTTPError(t *testing.T) {
	srv := testServer(t)
	f := Fetcher{Client: testClient(), Timeout: 100 * time.Millisecond}

	results := collectResults(f.FetchURLs(context.Background(), []string{srv.URL + "/error"}))
	require.Len(t, results, 1)
	assert.ErrorContains(t, results[0].Err, "HTTP 500")
}

func TestFetchURLs_MixedResults(t *testing.T) {
	srv := testServer(t)
	f := Fetcher{Client: testClient(), Parallel: 2, Timeout: 100 * time.Millisecond}

	urls := []string{srv.URL + "/ok", srv.URL + "/error"}
	results := collectResults(f.FetchURLs(context.Background(), urls))
	require.Len(t, results, 2)

	// Sort by error presence for deterministic assertions.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Err == nil && results[j].Err != nil
	})

	assert.NoError(t, results[0].Err)
	assert.Contains(t, results[0].Doc.Markdown, "Hello world")
	assert.ErrorContains(t, results[1].Err, "HTTP 500")
}

func TestFetchURLs_AppliesOverrides(t *testing.T) {
	srv := testServer(t)
	f := Fetcher{
		Client:  testClient(),
		Timeout: 5 * time.Second,
		Overrides: Overrides{
			Title:  "Custom Title",
			Source: "test",
			Tags:   []string{"a", "b"},
		},
	}

	results := collectResults(f.FetchURLs(context.Background(), []string{srv.URL + "/ok"}))
	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	assert.Equal(t, "Custom Title", results[0].Doc.Frontmatter.Title)
	assert.Equal(t, "test", results[0].Doc.Frontmatter.Source)
	assert.Equal(t, []string{"a", "b"}, results[0].Doc.Frontmatter.Tags)
}

func TestFetchURLs_OverridesAppliedOnError(t *testing.T) {
	srv := testServer(t)
	f := Fetcher{
		Client:    testClient(),
		Timeout:   100 * time.Millisecond,
		Overrides: Overrides{Title: "Override Title"},
	}

	results := collectResults(f.FetchURLs(context.Background(), []string{srv.URL + "/error"}))
	require.Len(t, results, 1)
	assert.Error(t, results[0].Err)
	// Overrides are applied to the partial Doc so stubs carry the known info.
	assert.Equal(t, "Override Title", results[0].Doc.Frontmatter.Title)
	assert.Equal(t, srv.URL+"/error", results[0].Doc.Frontmatter.URL)
}

func TestFetchURLs_ContextCancellation(t *testing.T) {
	srv := testServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	f := Fetcher{Client: testClient(), Timeout: 5 * time.Second}
	results := collectResults(f.FetchURLs(ctx, []string{srv.URL + "/ok"}))
	require.Len(t, results, 1)
	assert.Error(t, results[0].Err)
}

func TestHTML_RetriesOnError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		attempts := 0
		c := Client{
			RetryBackoff: time.Second,
			HTTPClient: &http.Client{
				Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
					attempts++
					if attempts < 3 {
						return &http.Response{
							StatusCode: http.StatusInternalServerError,
							Body:       io.NopCloser(strings.NewReader("transient")),
						}, nil
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(testHTML)),
					}, nil
				}),
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		html, err := c.HTML(ctx, "http://fake/test")
		require.NoError(t, err)
		assert.Contains(t, html, "Test Page")
		assert.Equal(t, 3, attempts)
	})
}

func TestHTML_RetriesUntilTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		attempts := 0
		c := Client{
			RetryBackoff: time.Second,
			HTTPClient: &http.Client{
				Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
					attempts++
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Body:       io.NopCloser(strings.NewReader("broken")),
					}, nil
				}),
			},
		}

		// With 1s backoff doubling: attempts at t=0, 1s, 3s. Timeout at 5s
		// expires during the 4s backoff wait.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := c.HTML(ctx, "http://fake/test")
		assert.ErrorContains(t, err, "HTTP 500")
		assert.Equal(t, 3, attempts)
	})
}

func TestHTML_ContextCancelledDuringBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		attempts := 0
		c := Client{
			RetryBackoff: time.Second,
			HTTPClient: &http.Client{
				Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
					attempts++
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Body:       io.NopCloser(strings.NewReader("broken")),
					}, nil
				}),
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error, 1)
		go func() {
			_, err := c.HTML(ctx, "http://fake/test")
			done <- err
		}()

		// Wait for the first attempt and backoff to start.
		synctest.Wait()
		assert.Equal(t, 1, attempts)

		// Cancel during backoff.
		cancel()
		synctest.Wait()

		err := <-done
		assert.ErrorContains(t, err, "HTTP 500")
	})
}

func TestHTML_ExponentialBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var timestamps []time.Duration
		start := time.Now()

		c := Client{
			RetryBackoff:    time.Second,
			MaxRetryBackoff: 20 * time.Second,
			HTTPClient: &http.Client{
				Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
					timestamps = append(timestamps, time.Since(start))
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Body:       io.NopCloser(strings.NewReader("broken")),
					}, nil
				}),
			},
		}

		// Timeout at 5s: attempts at t=0, 1s, 3s. The 4s backoff exceeds timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, _ = c.HTML(ctx, "http://fake/test")

		require.Len(t, timestamps, 3)
		assert.Equal(t, time.Duration(0), timestamps[0])
		assert.Equal(t, time.Second, timestamps[1])
		assert.Equal(t, 3*time.Second, timestamps[2])
	})
}

func TestFetchURLs_EmptyURLs(t *testing.T) {
	f := Fetcher{Timeout: 5 * time.Second}
	results := collectResults(f.FetchURLs(context.Background(), nil))
	assert.Empty(t, results)
}

func TestFetchURLs_DefaultParallel(t *testing.T) {
	srv := testServer(t)
	// Parallel=0 should default to 1, not panic.
	f := Fetcher{Client: testClient(), Parallel: 0, Timeout: 5 * time.Second}
	results := collectResults(f.FetchURLs(context.Background(), []string{srv.URL + "/ok"}))
	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
}
