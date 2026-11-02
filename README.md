# Expense Tracker

A small command-line expense tracker written in Go, built as a learning project.
Amounts are whole rupiah. Data is stored in a local SQLite database.

## Build

```powershell
go build -o expense.exe ./cmd/expense
```

## Usage

```powershell
# Add an expense (--date defaults to today)
.\expense.exe add --amount 25000 --category food --note "lunch"
.\expense.exe add --amount 150000 --category transport --date yesterday
.\expense.exe add --amount 1200000 --category bills --date 2026-10-05

# List expenses, newest first; filters are optional and can be combined
.\expense.exe list
.\expense.exe list --month 2026-10
.\expense.exe list --month last --category food

# Delete an expense (shows it and asks [y/N] first)
.\expense.exe delete 3

# Total per category for a month (default: current month)
.\expense.exe summary
.\expense.exe summary --month last

# Categories: list them, or add your own
.\expense.exe categories
.\expense.exe categories add eating out
```

### Input formats

| Input | Accepted values |
|---|---|
| `--amount` | positive whole number, e.g. `25000` (no decimals or separators) |
| `--date` | `YYYY-MM-DD`, `today`, `yesterday` |
| `--month` | `YYYY-MM`, `current`, `last` |
| `--category` | a name from `expense categories` (case-insensitive) |

The starting categories are: bills, entertainment, food, health, other, shopping, transport.

### Example output

```
> expense summary

November 2026

CATEGORY    AMOUNT  COUNT
transport  150,000      1
shopping    90,000      1
food        43,000      2
TOTAL      283,000      4
```

## Where the data lives

The database is created on first use at:

```
%AppData%\expense-tracker\expenses.db
```

## Project layout

```
cmd/expense/       entry point: wires up the database and runs the CLI
internal/expense/  domain logic: Expense type, money and date parsing, validation
internal/store/    SQLite: schema migrations and all SQL
internal/cli/      Cobra commands (add, list, delete, summary, categories)
```

Dependencies point one way: `cli` -> `store` -> `expense`.

## Development

```powershell
go build ./...
go test ./...
go vet ./...
gofmt -l .      # lists files that need formatting; gofmt -w . fixes them
```

Schema changes go in `internal/store/migrate.go` as a new entry at the end of
`migrations`. Never edit an entry that has already shipped.
