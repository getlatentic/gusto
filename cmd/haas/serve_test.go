package main

import "testing"

func TestASettingWrittenAsOneLineIsReadAsAList(t *testing.T) {
	t.Setenv("HAAS_MODELS", " platform/qwen.qwen3-vl-235b-a22b-instruct , ollama/llama3.2:3b ,, ")
	models := values("HAAS_MODELS")
	if len(models) != 2 || models[0] != "platform/qwen.qwen3-vl-235b-a22b-instruct" {
		t.Fatalf("got %v", models)
	}
}

func TestASettingNobodySetIsAnEmptyList(t *testing.T) {
	t.Setenv("HAAS_ALLOWED", "")
	if allowed := values("HAAS_ALLOWED"); len(allowed) != 0 {
		t.Fatalf("got %v", allowed)
	}
}
