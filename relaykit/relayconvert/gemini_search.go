package relayconvert

import (
	"fmt"
	"github.com/QuantumNous/new-api/relaykit/dto"
	geminichat "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/gemini_chat"
	oaichat "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/oai_chat"
)

func AddGeminiSearchToResponses(out *dto.OpenAIResponsesResponse, source *dto.GeminiChatResponse) {
	if out == nil {
		return
	}
	if queries := geminichat.GroundingWebSearchQueries(source); len(queries) > 0 {
		out.Output = append(out.Output, oaichat.WebSearchOutput(queries, fmt.Sprintf("ws_%s_%d", out.ID, len(out.Output))))
	}
}
func (s *ResponseStreamState) ObserveGeminiSearch(source *dto.GeminiChatResponse) {
	if s == nil {
		return
	}
	if s.searchSeen == nil {
		s.searchSeen = make(map[string]struct{})
	}
	for _, q := range geminichat.GroundingWebSearchQueries(source) {
		if _, ok := s.searchSeen[q]; !ok {
			s.searchSeen[q] = struct{}{}
			s.searchQueries = append(s.searchQueries, q)
		}
	}
}
