INSERT INTO enquiries (requester_branch_id, requester_staff_id, raw_request, medication_id, quantity, status, handled_by, wake_trigger)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, requester_branch_id, requester_staff_id, raw_request, medication_id, quantity, status, outcome, outcome_branch_id, handled_by, clarification_question, clarification_answer, wake_trigger, next_wake_at, created_at, updated_at, closed_at
