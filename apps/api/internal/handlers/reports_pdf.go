package handlers

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/phpdave11/gofpdf"

	"triageguard/apps/api/internal/db"
)

const reportFontFamily = "triageguard"

func (a *App) handleRequestsPDF(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	hasEnterprise, err := a.workspaceHasEnterprise(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !hasEnterprise {
		jsonResponse(w, http.StatusPaymentRequired, map[string]any{
			"error": "Enterprise plan required for PDF exports.",
		})
		return
	}

	windowDays := parseWindowDays(r.URL.Query().Get("days"))
	rows, err := a.store.ListRequestReportRows(r.Context(), workspace.ID, windowDays)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	lookup := a.buildRequestReportLookup(r.Context(), workspace.ID, rows)

	pdf, err := newReportPDF()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	renderRequestsPDF(pdf, workspace.Name, windowDays, rows, lookup)
	filename := fmt.Sprintf("triageguard-requests-%dd.pdf", windowDays)
	if err := writePDFResponse(w, pdf, filename); err != nil {
		a.logger.Printf("requests pdf write error workspace=%s: %v", workspace.ID, err)
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": "failed to generate requests PDF"})
		return
	}
}

func (a *App) handleAnalyticsPDF(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	hasEnterprise, err := a.workspaceHasEnterprise(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !hasEnterprise {
		jsonResponse(w, http.StatusPaymentRequired, map[string]any{
			"error": "Enterprise plan required for PDF exports.",
		})
		return
	}

	windowDays := parseWindowDays(r.URL.Query().Get("days"))
	timezone := "UTC"
	if policy, policyErr := a.store.GetPolicy(r.Context(), workspace.ID); policyErr == nil && strings.TrimSpace(policy.Timezone) != "" {
		if _, tzErr := time.LoadLocation(policy.Timezone); tzErr == nil {
			timezone = policy.Timezone
		}
	}
	analytics, err := a.store.RequestAnalytics(r.Context(), workspace.ID, windowDays, timezone)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	pdf, err := newReportPDF()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	renderAnalyticsPDF(pdf, workspace.Name, timezone, analytics)
	filename := fmt.Sprintf("triageguard-analytics-%dd.pdf", windowDays)
	if err := writePDFResponse(w, pdf, filename); err != nil {
		a.logger.Printf("analytics pdf write error workspace=%s: %v", workspace.ID, err)
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": "failed to generate analytics PDF"})
		return
	}
}

func writePDFResponse(w http.ResponseWriter, pdf *gofpdf.Fpdf, filename string) error {
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Cache-Control", "no-store")
	_, err := w.Write(out.Bytes())
	return err
}

func newReportPDF() (*gofpdf.Fpdf, error) {
	fontBytes, fontPath, err := loadReportFontBytes()
	if err != nil {
		return nil, err
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(12, 12, 12)
	pdf.SetAutoPageBreak(true, 12)
	pdf.SetCreator("TriageGuard", true)
	pdf.SetAuthor("TriageGuard", true)
	pdf.SetTitle("TriageGuard Export", true)
	pdf.AddUTF8FontFromBytes(reportFontFamily, "", fontBytes)
	if pdf.Err() {
		return nil, fmt.Errorf("failed to load report font from %s: %w", fontPath, pdf.Error())
	}
	pdf.SetFont(reportFontFamily, "", 10)
	return pdf, nil
}

func loadReportFontBytes() ([]byte, string, error) {
	candidates := make([]string, 0, 12)
	addCandidate := func(path string) {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			return
		}
		candidates = append(candidates, trimmed)
	}

	addCandidate(os.Getenv("TG_PDF_FONT_PATH"))
	addCandidate(os.Getenv("PDF_FONT_PATH"))
	addCandidate("internal/assets/fonts/NotoSans-Regular.ttf")
	addCandidate("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf")
	addCandidate("/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf")
	addCandidate("/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf")
	addCandidate("/System/Library/Fonts/Supplemental/Arial Unicode.ttf")
	addCandidate("/System/Library/Fonts/Supplemental/Arial.ttf")
	addCandidate("/Library/Fonts/Arial Unicode.ttf")
	addCandidate("/Library/Fonts/Arial Unicode MS.ttf")
	addCandidate("/Library/Fonts/Arial.ttf")
	addCandidate("C:\\Windows\\Fonts\\arialuni.ttf")
	addCandidate("C:\\Windows\\Fonts\\arial.ttf")

	for _, candidate := range candidates {
		resolved := candidate
		if !filepath.IsAbs(candidate) {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				continue
			}
			resolved = abs
		}
		fontBytes, err := os.ReadFile(resolved)
		if err != nil {
			continue
		}
		return fontBytes, resolved, nil
	}

	return nil, "", fmt.Errorf(
		"pdf unicode font not found. Set TG_PDF_FONT_PATH (tried %d common paths)",
		len(candidates),
	)
}

func renderReportHeader(pdf *gofpdf.Fpdf, title, subtitle string) {
	pdf.AddPage()
	pdf.SetFillColor(243, 246, 250)
	pdf.SetDrawColor(219, 226, 235)
	pdf.SetLineWidth(0.2)
	x, y := pdf.GetXY()
	w := 210.0 - 24.0
	h := 26.0
	pdf.Rect(x, y, w, h, "DF")

	pdf.SetXY(x+4, y+5)
	pdf.SetTextColor(15, 23, 42)
	pdf.SetFontSize(16)
	pdf.CellFormat(w-8, 6, title, "", 1, "L", false, 0, "")
	pdf.SetX(x + 4)
	pdf.SetTextColor(71, 85, 105)
	pdf.SetFontSize(9)
	pdf.MultiCell(w-8, 4.5, subtitle, "", "L", false)
	pdf.Ln(3)
	pdf.SetTextColor(17, 24, 39)
	pdf.SetFontSize(10)
}

func renderRequestsPDF(pdf *gofpdf.Fpdf, workspaceName string, windowDays int, rows []db.RequestReportRow, lookup requestReportLookup) {
	renderReportHeader(
		pdf,
		"TriageGuard Requests Report",
		fmt.Sprintf("Workspace: %s\nWindow: Last %d days\nGenerated UTC: %s",
			strings.TrimSpace(workspaceName),
			windowDays,
			time.Now().UTC().Format(time.RFC3339),
		),
	)

	if len(rows) == 0 {
		pdf.SetTextColor(71, 85, 105)
		pdf.MultiCell(0, 5.5, "No requests found for the selected period.", "", "L", false)
		return
	}

	cardW := 210.0 - 24.0
	for idx, row := range rows {
		renderRequestCard(pdf, idx, cardW, row, lookup)
	}
}

func renderRequestCard(pdf *gofpdf.Fpdf, idx int, cardW float64, row db.RequestReportRow, lookup requestReportLookup) {
	threadURL := ""
	if row.ThreadURL != nil && strings.TrimSpace(*row.ThreadURL) != "" {
		threadURL = strings.TrimSpace(*row.ThreadURL)
	}
	linearURL := ""
	if row.LinearIssueURL != nil && strings.TrimSpace(*row.LinearIssueURL) != "" {
		linearURL = strings.TrimSpace(*row.LinearIssueURL)
	}

	title := "-"
	if row.Title != nil && strings.TrimSpace(*row.Title) != "" {
		title = strings.TrimSpace(*row.Title)
	}
	titleLines := pdf.SplitText("Title: "+title, cardW-10)
	if len(titleLines) == 0 {
		titleLines = []string{"Title: -"}
	}
	titleHeight := float64(len(titleLines)) * 4.2
	metaHeight := 9.0
	if threadURL != "" {
		metaHeight += 3.8
	}
	if linearURL != "" {
		metaHeight += 3.8
	}
	headerHeight := 11.0
	subHeaderHeight := 6.0
	cardH := headerHeight + subHeaderHeight + titleHeight + metaHeight + 3.0

	ensureSpaceForBlock(pdf, cardH+3)
	x, y := pdf.GetXY()
	fill := idx%2 == 0
	if fill {
		pdf.SetFillColor(248, 250, 252)
		pdf.SetDrawColor(226, 232, 240)
		pdf.Rect(x, y, cardW, cardH, "DF")
	} else {
		pdf.SetFillColor(255, 255, 255)
		pdf.SetDrawColor(226, 232, 240)
		pdf.Rect(x, y, cardW, cardH, "D")
	}

	pdf.SetXY(x+3, y+3.8)
	pdf.SetTextColor(30, 41, 59)
	pdf.SetFontSize(10)
	pdf.CellFormat(cardW-6, 4.5,
		fmt.Sprintf("%s    Status: %s    Priority: %s",
			row.CreatedAt.UTC().Format("2006-01-02 15:04"),
			strings.TrimSpace(row.Status),
			strings.TrimSpace(row.Priority),
		),
		"", 1, "L", false, 0, "")

	owner := lookup.ownerDisplay(row.OwnerSlackID)
	pdf.SetX(x + 3)
	pdf.SetTextColor(71, 85, 105)
	pdf.SetFontSize(9)
	pdf.CellFormat(cardW-6, 4.2,
		fmt.Sprintf("Channel: %s    Owner: %s", lookup.channelDisplay(row.ChannelID), owner),
		"", 1, "L", false, 0, "")

	pdf.SetX(x + 3)
	pdf.SetTextColor(15, 23, 42)
	pdf.SetFontSize(9.5)
	pdf.MultiCell(cardW-6, 4.2, "Title: "+title, "", "L", false)

	linear := linearIssueLabel(row.LinearIssueURL, row.LinearIssueID)
	pdf.SetX(x + 3)
	pdf.SetTextColor(100, 116, 139)
	pdf.SetFontSize(8.5)
	pdf.MultiCell(cardW-6, 3.8,
		fmt.Sprintf("Due: %s    Ack: %s    Assign: %s    Resolved: %s    Linear: %s",
			formatPDFTime(row.DueAt),
			formatPDFTime(row.AckedAt),
			formatPDFTime(row.AssignedAt),
			formatPDFTime(row.ResolvedAt),
			linear,
		),
		"", "L", false,
	)
	if threadURL != "" {
		pdf.SetX(x + 3)
		pdf.SetTextColor(59, 130, 246)
		pdf.CellFormat(cardW-6, 3.8, "Thread: "+threadURL, "", 1, "L", false, 0, threadURL)
	}
	if linearURL != "" {
		pdf.SetX(x + 3)
		pdf.SetTextColor(59, 130, 246)
		pdf.CellFormat(cardW-6, 3.8, "Linear URL: "+linearURL, "", 1, "L", false, 0, linearURL)
	}

	pdf.SetY(y + cardH + 3)
	pdf.SetX(x)
}

func renderAnalyticsPDF(pdf *gofpdf.Fpdf, workspaceName, timezone string, analytics db.RequestAnalytics) {
	renderReportHeader(
		pdf,
		"TriageGuard Analytics Report",
		fmt.Sprintf("Workspace: %s\nWindow: Last %d days\nTimezone: %s\nGenerated UTC: %s",
			strings.TrimSpace(workspaceName),
			analytics.WindowDays,
			timezone,
			time.Now().UTC().Format(time.RFC3339),
		),
	)

	renderAnalyticsSummary(pdf, analytics)
	renderAnalyticsTable(pdf, analytics.Trend)
}

func renderAnalyticsSummary(pdf *gofpdf.Fpdf, analytics db.RequestAnalytics) {
	startX, startY := pdf.GetXY()
	boxW := (210.0 - 24.0 - 6.0) / 2
	boxH := 18.0

	drawMetricBox := func(x, y float64, title string, avg float64, samples int) {
		pdf.SetFillColor(248, 250, 252)
		pdf.SetDrawColor(226, 232, 240)
		pdf.Rect(x, y, boxW, boxH, "DF")
		pdf.SetXY(x+3, y+4)
		pdf.SetTextColor(71, 85, 105)
		pdf.SetFontSize(8.5)
		pdf.CellFormat(boxW-6, 4, title, "", 1, "L", false, 0, "")
		pdf.SetX(x + 3)
		pdf.SetTextColor(15, 23, 42)
		pdf.SetFontSize(12)
		pdf.CellFormat(boxW-6, 5.5, formatMetricMinutes(avg, samples), "", 1, "L", false, 0, "")
		pdf.SetX(x + 3)
		pdf.SetTextColor(100, 116, 139)
		pdf.SetFontSize(8.5)
		pdf.CellFormat(boxW-6, 3.5, fmt.Sprintf("%d samples", samples), "", 0, "L", false, 0, "")
	}

	drawMetricBox(startX, startY, "Average time to Ack", analytics.Ack.AvgMinutes, analytics.Ack.SampleSize)
	drawMetricBox(startX+boxW+6, startY, "Average time to Resolve", analytics.Resolve.AvgMinutes, analytics.Resolve.SampleSize)

	pdf.SetY(startY + boxH + 5)
	pdf.SetX(startX)
}

func renderAnalyticsTable(pdf *gofpdf.Fpdf, trend []db.RequestTrendPoint) {
	if len(trend) == 0 {
		pdf.SetTextColor(71, 85, 105)
		pdf.SetFontSize(9.5)
		pdf.MultiCell(0, 5.5, "No trend points available for the selected period.", "", "L", false)
		return
	}

	cols := []float64{34, 36, 30, 40, 34}
	headers := []string{"Date", "Ack avg (min)", "Ack samples", "Resolve avg (min)", "Resolve samples"}
	rowH := 6.5

	drawHeader := func() {
		pdf.SetFontSize(8.8)
		pdf.SetTextColor(30, 41, 59)
		pdf.SetFillColor(241, 245, 249)
		pdf.SetDrawColor(226, 232, 240)
		for i, header := range headers {
			pdf.CellFormat(cols[i], rowH, header, "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)
	}

	drawHeader()
	for idx, point := range trend {
		ensureSpaceForBlock(pdf, rowH)
		if idx > 0 && mathAbs(pdf.GetY()-12) < 0.1 {
			drawHeader()
		}
		fill := idx%2 == 0
		pdf.SetFontSize(8.6)
		pdf.SetTextColor(51, 65, 85)
		pdf.SetFillColor(255, 255, 255)
		if fill {
			pdf.SetFillColor(250, 252, 255)
		}
		pdf.SetDrawColor(226, 232, 240)

		values := []string{
			point.Date,
			formatOptionalPDFFloat(point.AckAvgMinutes),
			strconv.Itoa(point.AckSampleSize),
			formatOptionalPDFFloat(point.ResolveAvgMinutes),
			strconv.Itoa(point.ResolveSampleSize),
		}
		for i, v := range values {
			pdf.CellFormat(cols[i], rowH, v, "1", 0, "C", fill, 0, "")
		}
		pdf.Ln(-1)
	}
}

func ensureSpaceForBlock(pdf *gofpdf.Fpdf, neededHeight float64) {
	_, pageH := pdf.GetPageSize()
	_, _, _, bottom := pdf.GetMargins()
	if pdf.GetY()+neededHeight > pageH-bottom {
		pdf.AddPage()
	}
}

func formatPDFTime(v *time.Time) string {
	if v == nil {
		return "-"
	}
	return v.UTC().Format("2006-01-02 15:04")
}

func formatOptionalPDFFloat(v *float64) string {
	if v == nil {
		return "-"
	}
	return strconv.FormatFloat(*v, 'f', 2, 64)
}

func formatMetricMinutes(avg float64, sampleSize int) string {
	if sampleSize == 0 {
		return "-"
	}
	if avg >= 60 {
		return fmt.Sprintf("%.1fh", avg/60.0)
	}
	return fmt.Sprintf("%.1fm", avg)
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
