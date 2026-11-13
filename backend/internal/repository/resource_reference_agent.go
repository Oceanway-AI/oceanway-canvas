package repository

import "strings"

// Read retained native documents, not the bounded hydrated journal window or
// expiring execution leases. This does not interpret or rewrite Pi checkpoints.
func (r *Repository) agentResourceReferenceDocuments(userID string, resourceIDs []string) ([]ResourceReferenceDocument, error) {
	result := []ResourceReferenceDocument{}
	for _, source := range []struct {
		table, column, join, kind string
		jsonl                     bool
	}{
		{"cloud_agent_executions e", "e.state_json", "", "Agent 会话", false},
		{"cloud_agent_message_records m", "m.message_json", "m", "Agent 消息", false},
		{"cloud_agent_event_records m", "m.event_json", "m", "Agent 事件", false},
		{"cloud_agent_canvas_mutations m", "m.before_json", "m", "Agent 撤销历史", false},
		{"cloud_agent_pi_sessions m", "m.session_jsonl", "m", "Agent Pi 会话", true},
	} {
		query := r.db.Table(source.table).Select("e.id, e.title, "+source.column+" AS document").Where("e.user_id = ?", userID)
		if source.join != "" {
			query = query.Joins("JOIN cloud_agent_executions e ON e.id = m.run_id AND e.user_id = m.user_id")
		}
		// Reduce reads for small cleanup sets; exact resource fields are still
		// interpreted by the shared collector, never by this substring prefilter.
		if len(resourceIDs) <= 100 {
			conditions := make([]string, 0, len(resourceIDs))
			args := make([]any, 0, len(resourceIDs))
			for _, id := range resourceIDs {
				conditions = append(conditions, source.column+" LIKE ?")
				args = append(args, "%"+id+"%")
			}
			query = query.Where(strings.Join(conditions, " OR "), args...)
		}
		rows, err := query.Rows()
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, title, document string
			if err := rows.Scan(&id, &title, &document); err != nil {
				_ = rows.Close()
				return nil, err
			}
			documents := []string{document}
			if source.jsonl {
				documents = strings.Split(document, "\n")
			}
			for _, raw := range documents {
				if strings.TrimSpace(raw) != "" {
					result = append(result, ResourceReferenceDocument{Kind: source.kind, ID: id, Title: title, PrimaryJSON: raw})
				}
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
