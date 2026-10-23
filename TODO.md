# TODO

Notes for later work on rowx. Found while using rowx in invoicepit
(`plugins/invoices/handlers_test.go`, `TestCreditNoteRefersToInvoice`,
2026-10-24).

## Insert ignores the defaults of the database

`Rx.Insert` inserts every column which is not tagged `auto`, with the zero
value of its field when the field is not set. So the `DEFAULT` of a column
never applies. Example: table `invoices` has
`status VARCHAR(20) NOT NULL DEFAULT 'pending'` and
`CHECK (status IN ('pending', 'paid', 'overdue', 'voided'))`. An
`Invoices{...}` without `Status` is inserted with `status = ''`, and the
CHECK rejects the row. The caller must repeat every default in Go.

Possible solutions:

- a tag option (e.g. `rx:"status,default"`) for columns which are left out
  of the INSERT when the field has its zero value;
- `rowx generate` could read the defaults from the schema and set them in a
  constructor, or add the tag option above for columns with a `DEFAULT`.

## No way to run outside a transaction on one connection

`WithTx` accepts only `*sqlx.Tx`. Some statements must run on one connection
but outside a transaction, e.g. `PRAGMA foreign_keys=ON`, which SQLite
ignores inside a transaction and which is per connection. In the test above
the PRAGMA is executed on a `*sqlx.Conn` (`rx.DB().Connx(ctx)`), and the
`*sqlx.Tx` from `conn.BeginTxx` is then given to `WithTx`.

Possible solution: let the queryer be any `Ext` (e.g. `WithQueryer(Ext)`), so
that a `*sqlx.Conn` or a `*sqlx.DB` can be used too. Note that `*sqlx.Conn`
does not implement `sqlx.Ext` as it is (it has the `...Context` methods), so
it would need a small adapter.
