package proxy

import "sort"

// known is where gusto's own deployments reach their model providers, with no key: the address, how the provider is let in, the
// shapes it answers and the most it writes in one answer. Another program that sends calls through the proxy takes a provider
// from here and adds the key it holds, and keeps no copy of these.
var known = map[string]Credential{
	// Bedrock's Mantle serves Gemma 4 under /openai, and Gemma answers Chat Completions alone, so Messages and Responses calls are translated.
	"gemma": {Address: "https://bedrock-mantle.us-east-1.api.aws/openai", Auth: Bearer, Wires: []string{"chat"}, MaxOutput: 8192},
	// The rest of Mantle's models, which answer what a bearer token says.
	"platform": {Address: "https://bedrock-mantle.us-east-1.api.aws", Auth: Bearer},
	// DeepSeek answers Chat Completions and Responses at its root, and Messages at /anthropic.
	"deepseek":  {Address: "https://api.deepseek.com", Auth: Bearer},
	"anthropic": {Address: "https://api.deepseek.com/anthropic", Auth: Anthropic},
}

// Known is the provider of that name as gusto's deployments reach it, with no key, or false when gusto has none by that name.
func Known(name string) (Credential, bool) {
	credential, found := known[name]
	if !found {
		return Credential{}, false
	}
	credential.Name = name
	credential.Wires = append([]string(nil), credential.Wires...)
	return credential, true
}

// KnownNames lists the providers Known has.
func KnownNames() []string {
	names := make([]string, 0, len(known))
	for name := range known {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// WithKey is the credential holding that key.
func (c Credential) WithKey(key string) Credential {
	c.Key = key
	return c
}
