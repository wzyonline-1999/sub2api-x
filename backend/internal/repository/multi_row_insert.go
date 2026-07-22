package repository

import (
	"strconv"
	"strings"
)

// buildMultiRowInsertQuery builds one parameterized INSERT statement for a
// trusted table/column list. Callers normalize and filter rows before passing
// them here so the resulting statement remains atomic for the entire batch.
func buildMultiRowInsertQuery(table, columns string, rows [][]any) (string, []any) {
	if len(rows) == 0 {
		return "", nil
	}

	columnCount := len(rows[0])
	args := make([]any, 0, len(rows)*columnCount)

	var query strings.Builder
	query.Grow(len(table) + len(columns) + len(rows)*columnCount*4)
	_, _ = query.WriteString("INSERT INTO ")
	_, _ = query.WriteString(table)
	_, _ = query.WriteString(" (")
	_, _ = query.WriteString(columns)
	_, _ = query.WriteString(") VALUES ")

	placeholder := 1
	for rowIndex, row := range rows {
		if rowIndex > 0 {
			_ = query.WriteByte(',')
		}
		_ = query.WriteByte('(')
		for columnIndex := range row {
			if columnIndex > 0 {
				_ = query.WriteByte(',')
			}
			_ = query.WriteByte('$')
			_, _ = query.WriteString(strconv.Itoa(placeholder))
			placeholder++
		}
		_ = query.WriteByte(')')
		args = append(args, row...)
	}

	return query.String(), args
}
