// Package migrations содержит SQL-миграции, встроенные в бинарник.
package migrations

import "embed"

// FS — файлы миграций. Встраиваются при сборке, поэтому бинарнику
// не нужны никакие внешние файлы при деплое.
//
//go:embed *.sql
var FS embed.FS
