UPDATE enquiries
SET medication_id = $2,
    status = $3,
    outcome = $4,
    outcome_branch_id = $5,
    clarification_question = $6,
    clarification_answer = $7,
    wake_trigger = $8,
    next_wake_at = $9,
    closed_at = $10,
    updated_at = now()
WHERE id = $1
RETURNING id, requester_branch_id, requester_staff_id, raw_request, medication_id, quantity, status, outcome, outcome_branch_id, handled_by, clarification_question, clarification_answer, wake_trigger, next_wake_at, created_at, updated_at, closed_at
