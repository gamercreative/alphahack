SELECT id, enquiry_id, trigger, model, input_tokens, output_tokens, steps, cost_micros, status, error, started_at, finished_at
FROM enquiry_agent_runs
WHERE enquiry_id = $1
ORDER BY started_at, id
