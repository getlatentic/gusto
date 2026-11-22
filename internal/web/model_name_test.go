package web

import "testing"

func TestAModelIsNamedWithoutItsCredentialOrVendorAndKeepsItsVersion(t *testing.T) {
	for id, want := range map[string]string{
		"meta/muse-spark-1.3":                    "Muse Spark 1.3",
		"gemma/google.gemma-4-26b-a4b":           "Gemma 4 26b A4b",
		"bedrock/us.anthropic.claude-sonnet-4-5": "Claude Sonnet 4 5",
		"deepseek/deepseek-flash":                "DeepSeek Flash",
		"openai/gpt-5.1":                         "Gpt 5.1",
		"Already Named":                          "Already Named",
	} {
		if got := modelName(id); got != want {
			t.Errorf("modelName(%q) = %q, want %q", id, got, want)
		}
	}
}
