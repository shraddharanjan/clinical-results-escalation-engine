package result

import (
	"context"
	"fmt"
)

const defaultReadLimit = 200

// ListForAPI returns the most recently received clinical results.
func (r *PostgresRepository) ListForAPI(
	ctx context.Context,
) ([]Result, error) {
	const query = `
		SELECT
			id,
			source_system,
			source_result_id,
			patient_reference,
			test_code,
			numeric_value,
			unit,
			reported_at,
			received_at,
			severity,
			matched_rule,
			raw_payload
		FROM clinical_results
		ORDER BY received_at DESC
		LIMIT $1
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		defaultReadLimit,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"query clinical results: %w",
			err,
		)
	}
	defer rows.Close()

	results := make([]Result, 0)

	for rows.Next() {
		var result Result

		if err := rows.Scan(
			&result.ID,
			&result.SourceSystem,
			&result.SourceResultID,
			&result.PatientReference,
			&result.TestCode,
			&result.NumericValue,
			&result.Unit,
			&result.ReportedAt,
			&result.ReceivedAt,
			&result.Severity,
			&result.MatchedRule,
			&result.RawPayload,
		); err != nil {
			return nil, fmt.Errorf(
				"scan clinical result: %w",
				err,
			)
		}

		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate clinical results: %w",
			err,
		)
	}

	return results, nil
}
