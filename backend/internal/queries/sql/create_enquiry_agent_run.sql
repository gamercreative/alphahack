INSERT INTO enquiry_agent_runs (enquiry_id, trigger, model)
VALUES ($1, $2, $3)
RETURNING id, enquiry_id, trigger, model, input_tokens, output_tokens, steps, cost_micros, status, error, started_at, finished_at
