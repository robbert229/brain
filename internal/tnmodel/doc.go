// Package tnmodel contains generated TaskNotes schema models.
//
//go:generate go run github.com/atombender/go-jsonschema@latest --schema-root-type=https://example.com/schemas/tasknotes-task-record.schema.json=TaskNote -p tnmodel -o tasknote.gen.go tasknote.schema.json
package tnmodel
