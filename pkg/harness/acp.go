package harness

// speaksACP fills in where the Agent Client Protocol says things, for whatever
// the adapter leaves unsaid. The words arrive as message chunks and a tool call
// is announced once and then updated, so each is read from the one update kind
// that carries it. The thinking arrives on a kind of its own and stays out of
// the transcript. Usage is the total a prompt's answer states, and a harness
// that states none leaves the count to the proxy.
func (a *Adapter) speaksACP() {
	settle := func(field *string, path string) {
		if *field == "" {
			*field = path
		}
	}
	settle(&a.Text, "params.update.content.text")
	settle(&a.Message, "params.update.messageId")
	settle(&a.ToolName, "params.update.title")
	settle(&a.ToolInput, "params.update.rawInput")
	settle(&a.ToolStatus, "params.update.status")
	settle(&a.TotalTokens, "result.usage.totalTokens")
	if a.When == nil {
		a.When = map[string]map[string]string{}
	}
	for purpose, kind := range map[string]string{"text": "agent_message_chunk", "tool": "tool_call"} {
		if a.When[purpose] == nil {
			a.When[purpose] = map[string]string{"params.update.sessionUpdate": kind}
		}
	}
}
