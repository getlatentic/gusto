CREATE INDEX usage_calls_latest ON usage_calls (called_at, task, line);
CREATE INDEX usage_calls_spend ON usage_calls (session, usd, input_tokens, output_tokens);
