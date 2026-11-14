INSERT INTO enquiry_agent_tool_calls (run_id, name, input, output, is_error, duration_ms)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, seq, run_id, name, input, output, is_error, duration_ms, created_at
