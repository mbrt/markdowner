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
	f := Fetcher{Client: testClient(), Timeout: 5 * time.Second}

	results := collectResults(f.FetchURLs(context.Background(), []string{srv.URL + "/error"}))
	require.Len(t, results, 1)
	assert.ErrorContains(t, results[0].Err, "HTTP 500")
}

func TestFetchURLs_MixedResults(t *testing.T) {
	srv := testServer(t)
	f := Fetcher{Client: testClient(), Parallel: 2, Timeout: 5 * time.Second}

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

func TestFetchURLs_OverridesNotAppliedOnError(t *testing.T) {
	srv := testServer(t)
	f := Fetcher{
		Client:    testClient(),
		Timeout:   5 * time.Second,
		Overrides: Overrides{Title: "Should Not Appear"},
	}

	results := collectResults(f.FetchURLs(context.Background(), []string{srv.URL + "/error"}))
	require.Len(t, results, 1)
	assert.Error(t, results[0].Err)
	assert.Empty(t, results[0].Doc.Frontmatter.Title)
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
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < retryAttempts {
			http.Error(w, "transient", http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, testHTML)
	}))
	t.Cleanup(srv.Close)

	c := testClient()
	html, err := c.HTML(context.Background(), srv.URL+"/")
	require.NoError(t, err)
	assert.Contains(t, html, "Test Page")
	assert.Equal(t, retryAttempts, attempts)
}

func TestHTML_ExhaustsRetries(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		http.Error(w, "always broken", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := testClient()
	_, err := c.HTML(context.Background(), srv.URL+"/")
	assert.ErrorContains(t, err, "HTTP 500")
	assert.Equal(t, retryAttempts, attempts)
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
		assert.ErrorIs(t, err, context.Canceled)
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
