package convmeta

import "strings"

// ResponsesToolState preserves the identity of custom tools encoded as Gemini
// functions. It belongs to one request, including its response conversion.
type ResponsesToolState struct {
	CustomToolNames map[string]struct{}
	Namespaces      map[string]string
	FunctionNames   map[string]struct{}
}

const DefaultResponsesToolNamespace = "functions"

func NamespacedToolName(namespace, name string) string {
	if namespace == "" || namespace == DefaultResponsesToolNamespace {
		return name
	}
	return namespace + "__" + name
}
func (s *ResponsesToolState) ResponsesToolName(name string) (string, string) {
	if s == nil {
		return "", name
	}
	ns := s.Namespaces[name]
	if ns == "" || ns == DefaultResponsesToolNamespace {
		return "", name
	}
	return ns, strings.TrimPrefix(name, ns+"__")
}

const CustomToolInputArgument = "input"

func (s *ResponsesToolState) IsCustomTool(name string) bool {
	if s == nil {
		return false
	}
	_, ok := s.CustomToolNames[name]
	return ok
}

type responsesToolsMeta interface {
	ResponsesToolState() *ResponsesToolState
	SetResponsesToolState(*ResponsesToolState)
}

func ResponsesToolStateOf(m Meta) *ResponsesToolState {
	if m, ok := m.(responsesToolsMeta); ok {
		return m.ResponsesToolState()
	}
	return nil
}
func SetResponsesToolState(m Meta, state *ResponsesToolState) {
	if m, ok := m.(responsesToolsMeta); ok {
		m.SetResponsesToolState(state)
	}
}
func (v *Values) ResponsesToolState() *ResponsesToolState {
	if v == nil {
		return nil
	}
	return v.ResponsesTools
}
func (v *Values) SetResponsesToolState(s *ResponsesToolState) {
	if v != nil {
		v.ResponsesTools = s
	}
}
