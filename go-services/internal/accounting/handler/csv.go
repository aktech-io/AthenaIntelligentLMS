package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/accounting/model"
)

// wantsCSV reports whether the client requested CSV output via ?format=csv.
func wantsCSV(r *http.Request) bool {
	return strings.EqualFold(r.URL.Query().Get("format"), "csv")
}

// writeCSV streams rows as a downloadable CSV attachment. Money values are
// expected to be plain decimal strings (no thousands separators) so the output
// is machine-parseable.
func writeCSV(w http.ResponseWriter, filename string, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	cw := csv.NewWriter(w)
	_ = cw.WriteAll(rows) // also flushes
}

// writeTrialBalanceCSV flattens a trial balance report to CSV and writes it.
func writeTrialBalanceCSV(w http.ResponseWriter, tb *model.TrialBalanceResponse) {
	rows := [][]string{{"AccountCode", "AccountName", "AccountType", "BalanceType", "Balance", "Currency", "PeriodYear", "PeriodMonth"}}
	for _, a := range tb.Accounts {
		rows = append(rows, []string{
			a.AccountCode,
			a.AccountName,
			a.AccountType,
			a.BalanceType,
			a.Balance.String(),
			a.Currency,
			strconv.Itoa(a.PeriodYear),
			strconv.Itoa(a.PeriodMonth),
		})
	}
	rows = append(rows,
		[]string{},
		[]string{"Total Debits", tb.TotalDebits.String()},
		[]string{"Total Credits", tb.TotalCredits.String()},
		[]string{"Balanced", strconv.FormatBool(tb.Balanced)},
	)
	writeCSV(w, fmt.Sprintf("trial-balance-%d-%02d.csv", tb.PeriodYear, tb.PeriodMonth), rows)
}

// writeCashFlowCSV flattens a cash flow statement to CSV and writes it.
func writeCashFlowCSV(w http.ResponseWriter, cf *model.CashFlowResponse) {
	rows := [][]string{{"Section", "Description", "Amount"}}
	appendItems := func(section string, items []model.CashFlowItem, total string) {
		for _, it := range items {
			rows = append(rows, []string{section, it.Description, it.Amount.String()})
		}
		rows = append(rows, []string{section, "Total " + section, total})
	}
	appendItems("Operating", cf.OperatingItems, cf.TotalOperating.String())
	appendItems("Investing", cf.InvestingItems, cf.TotalInvesting.String())
	appendItems("Financing", cf.FinancingItems, cf.TotalFinancing.String())
	rows = append(rows,
		[]string{"Summary", "Net Cash Flow", cf.NetCashFlow.String()},
		[]string{"Summary", "Opening Cash", cf.OpeningCash.String()},
		[]string{"Summary", "Closing Cash", cf.ClosingCash.String()},
	)
	writeCSV(w, fmt.Sprintf("cash-flow-%d-%02d.csv", cf.PeriodYear, cf.PeriodMonth), rows)
}

// writeLedgerCSV flattens an account ledger to CSV and writes it.
func writeLedgerCSV(w http.ResponseWriter, accountID uuid.UUID, lines []model.JournalLineResponse) {
	rows := [][]string{{"LineNo", "AccountCode", "AccountName", "Description", "Debit", "Credit", "Currency"}}
	for _, l := range lines {
		desc := ""
		if l.Description != nil {
			desc = *l.Description
		}
		rows = append(rows, []string{
			strconv.Itoa(l.LineNo),
			l.AccountCode,
			l.AccountName,
			desc,
			l.DebitAmount.String(),
			l.CreditAmount.String(),
			l.Currency,
		})
	}
	writeCSV(w, fmt.Sprintf("ledger-%s.csv", accountID), rows)
}
