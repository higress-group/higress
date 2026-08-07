package vector

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestESParseQueryResponseValidatesSourceFields(t *testing.T) {
	provider := &ESProvider{}

	results, err := provider.parseQueryResponse([]byte(`{"hits":{"total":{"value":1},"hits":[{"_id":"ok","_score":0.9,"_source":{"question":"q","answer":"a"}}]}}`), nil)
	require.NoError(t, err)
	require.Equal(t, []QueryResult{{Text: "q", Score: 0.9, Answer: "a"}}, results)

	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "missing question", body: `{"hits":{"total":{"value":1},"hits":[{"_id":"bad","_source":{"answer":"a"}}]}}`, want: `hit "bad" has missing or non-string question`},
		{name: "non-string answer", body: `{"hits":{"total":{"value":1},"hits":[{"_id":"bad","_source":{"question":"q","answer":42}}]}}`, want: `hit "bad" has missing or non-string answer`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results, err := provider.parseQueryResponse([]byte(tc.body), nil)
			require.Nil(t, results)
			require.ErrorContains(t, err, tc.want)
		})
	}
}
