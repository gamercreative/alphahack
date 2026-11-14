SELECT id, requester_branch_id, requester_staff_id, raw_request, medication_id, quantity, status, outcome, outcome_branch_id, handled_by, clarification_question, clarification_answer, wake_trigger, next_wake_at, created_at, updated_at, closed_at
FROM enquiries
WHERE id = $1
