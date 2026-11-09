package vector

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestChromaParseQueryResponseValid covers the happy path: a single-batch
// response whose ids, distances and documents are aligned. Passing a nil logger
// also checks that the parser tolerates a missing logger, mirroring how the
// Elasticsearch parser is exercised.
func TestChromaParseQueryResponseValid(t *testing.T) {
	provider := &ChromaProvider{}

	results, err := provider.parseQueryResponse(
		[]byte(`{"ids":[["a","b"]],"distances":[[0.1,0.2]],"documents":[["da","db"]]}`),
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, []QueryResult{
		{Text: "a", Score: 0.1, Answer: "da"},
		{Text: "b", Score: 0.2, Answer: "db"},
	}, results)
}

// TestChromaParseQueryResponseAcceptsLongerValueSlices pins the boundary of the
// length checks: distances/documents that carry at least as many entries as ids
// are valid, and only the first n entries line up with the returned ids.
func TestChromaParseQueryResponseAcceptsLongerValueSlices(t *testing.T) {
	provider := &ChromaProvider{}

	results, err := provider.parseQueryResponse(
		[]byte(`{"ids":[["a"]],"distances":[[0.1,0.9]],"documents":[["da","extra"]]}`),
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, []QueryResult{{Text: "a", Score: 0.1, Answer: "da"}}, results)
}

// TestChromaParseQueryResponseIgnoresUnrelatedFields makes sure the optional
// fields Chroma may include alongside ids (uris, data, included) do not affect
// parsing of the aligned ids/distances/documents.
func TestChromaParseQueryResponseIgnoresUnrelatedFields(t *testing.T) {
	provider := &ChromaProvider{}

	results, err := provider.parseQueryResponse(
		[]byte(`{"ids":[["a"]],"distances":[[0.1]],"documents":[["da"]],"uris":["u"],"data":[],"included":["distances","documents"]}`),
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, []QueryResult{{Text: "a", Score: 0.1, Answer: "da"}}, results)
}

// TestChromaParseQueryResponseValidatesShape is a regression test for the
// index-out-of-range panic that aborted the ai-cache HTTP callback and left the
// request paused. Chroma answers an unknown collection with an error body and a
// query that matched nothing with {"ids":[]}; both must yield an error instead
// of panicking. The distances/documents cases cover the `omitempty` fields,
// which may legitimately be absent or shorter than ids. Every case also asserts
// that no panic escapes and that results stay nil so the caller takes the error
// path that resumes the request.
func TestChromaParseQueryResponseValidatesShape(t *testing.T) {
	provider := &ChromaProvider{}

	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "empty ids array",
			body: `{"ids":[]}`,
			want: "[Chroma] no query results found in response",
		},
		{
			name: "ids omitted entirely",
			body: `{}`,
			want: "[Chroma] no query results found in response",
		},
		{
			name: "chroma error body",
			body: `{"error":"Collection [missing] does not exist"}`,
			want: "[Chroma] no query results found in response",
		},
		{
			name: "inner empty slice",
			body: `{"ids":[[]]}`,
			want: "[Chroma] no query results found in response",
		},
		{
			name: "null ids",
			body: `{"ids":null}`,
			want: "[Chroma] no query results found in response",
		},
		{
			name: "distances omitted",
			body: `{"ids":[["a"]],"documents":[["da"]]}`,
			want: "distances are missing or shorter than ids",
		},
		{
			name: "distances shorter than ids",
			body: `{"ids":[["a","b"]],"distances":[[0.1]],"documents":[["da","db"]]}`,
			want: "distances are missing or shorter than ids",
		},
		{
			name: "documents omitted",
			body: `{"ids":[["a"]],"distances":[[0.1]]}`,
			want: "documents are missing or shorter than ids",
		},
		{
			name: "documents shorter than ids",
			body: `{"ids":[["a","b"]],"distances":[[0.1,0.2]],"documents":[["da"]]}`,
			want: "documents are missing or shorter than ids",
		},
		{
			name: "invalid json body",
			body: `not json`,
		},
		{
			name: "empty body",
			body: ``,
		},
		{
			name: "whitespace body",
			body: `   `,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var (
				results []QueryResult
				err     error
			)
			require.NotPanics(t, func() {
				results, err = provider.parseQueryResponse([]byte(tc.body), nil)
			})
			require.Error(t, err)
			require.Nil(t, results)
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
}

// TestChromaProviderInitializerConfig checks the provider wiring around the
// parser: required fields are validated and the created provider reports the
// chroma provider type.
func TestChromaProviderInitializerConfig(t *testing.T) {
	initializer := &chromaProviderInitializer{}

	require.ErrorContains(t, initializer.ValidateConfig(ProviderConfig{}), "collectionID is required")
	require.ErrorContains(t, initializer.ValidateConfig(ProviderConfig{collectionID: "c"}), "serviceName is required")
	require.NoError(t, initializer.ValidateConfig(ProviderConfig{collectionID: "c", serviceName: "s"}))

	provider, err := initializer.CreateProvider(ProviderConfig{collectionID: "c", serviceName: "s"})
	require.NoError(t, err)
	require.Equal(t, PROVIDER_TYPE_CHROMA, provider.GetProviderType())
}
