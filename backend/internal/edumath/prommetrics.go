package edumath

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/model"
)

// Prometheus metrics for the education module (DEV-121, E95).
// GET /api/edu-metrics is off unless EDU_METRICS_TOKEN (or FLOW_METRICS_TOKEN)
// is set, and then requires that bearer token. Only aggregate gauges are
// exported: no tenant ids, job ids, answers or traces. Alert delivery and
// external probes stay with the deployment.

func metricsToken() string {
	if t := os.Getenv("EDU_METRICS_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("FLOW_METRICS_TOKEN")
}

func (h *API) promMetrics(c *gin.Context) {
	tok := metricsToken()
	if tok == "" {
		c.Status(http.StatusNotFound)
		return
	}
	if subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte("Bearer "+tok)) != 1 {
		c.Status(http.StatusUnauthorized)
		return
	}
	now := time.Now().UTC()
	if h.Svc != nil {
		now = h.Svc.now()
	}
	c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(PrometheusText(h.DB, now)))
}

// PrometheusText renders education gauges in Prometheus text format.
func PrometheusText(db *gorm.DB, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# HELP edu_up Education module is serving metrics.\n# TYPE edu_up gauge\nedu_up 1\n")

	type row struct {
		Status string
		N      int64
	}
	var rows []row
	db.Model(&model.EduMathJob{}).Select("status, count(*) as n").Group("status").Scan(&rows)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Status < rows[j].Status })
	fmt.Fprintf(&b, "# HELP edu_math_jobs Education math jobs by status.\n# TYPE edu_math_jobs gauge\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "edu_math_jobs{status=%q} %d\n", r.Status, r.N)
	}

	var stuck, reconcile, heldUnknown, unsettled, anomalies int64
	db.Model(&model.EduMathJob{}).Where("status IN ? AND updated_at < ?", openJobStatuses, now.Add(-stuckAfter)).Count(&stuck)
	db.Model(&model.EduMathJob{}).Where("status = ?", "reconciliation_required").Count(&reconcile)
	db.Model(&model.EduMathJob{}).Where("settlement = ?", "held_unknown").Count(&heldUnknown)
	db.Model(&model.EduMathJob{}).Where("status IN ? AND settlement = ? AND updated_at < ?", terminalJobStatuses, "open", now.Add(-unsettledAfter)).Count(&unsettled)
	db.Model(&model.EduLedgerAnomaly{}).Where("status <> ?", "resolved").Count(&anomalies)

	var oldest float64
	var first model.EduMathJob
	if db.Model(&model.EduMathJob{}).Where("status = ?", "queued").Order("created_at").Limit(1).Find(&first).RowsAffected > 0 {
		oldest = now.Sub(first.CreatedAt).Seconds()
		if oldest < 0 {
			oldest = 0
		}
	}

	// Failure ratio over the last 24 hours (same rule as /edu/observability).
	from := now.Add(-24 * time.Hour)
	var succeeded, failed int64
	db.Model(&model.EduMathJob{}).Where("status = ? AND updated_at >= ?", "succeeded", from).Count(&succeeded)
	db.Model(&model.EduMathJob{}).Where("status = ? AND updated_at >= ?", "failed", from).Count(&failed)
	terminal := succeeded + failed
	var ratio float64
	if terminal >= failureRateMinJobs {
		ratio = float64(failed) / float64(terminal)
	}

	fmt.Fprintf(&b, "# HELP edu_math_jobs_stuck Open jobs silent for more than 30 minutes.\n# TYPE edu_math_jobs_stuck gauge\nedu_math_jobs_stuck %d\n", stuck)
	fmt.Fprintf(&b, "# HELP edu_math_jobs_oldest_queued_seconds Age of the oldest queued education job.\n# TYPE edu_math_jobs_oldest_queued_seconds gauge\nedu_math_jobs_oldest_queued_seconds %d\n", int64(oldest))
	fmt.Fprintf(&b, "# HELP edu_math_jobs_reconciliation_required Jobs whose service outcome is unknown and need reconciliation.\n# TYPE edu_math_jobs_reconciliation_required gauge\nedu_math_jobs_reconciliation_required %d\n", reconcile)
	fmt.Fprintf(&b, "# HELP edu_math_jobs_unknown_cost Reservations held because the provider charge is unknown.\n# TYPE edu_math_jobs_unknown_cost gauge\nedu_math_jobs_unknown_cost %d\n", heldUnknown)
	fmt.Fprintf(&b, "# HELP edu_math_jobs_unsettled_reservations Finished jobs whose reservation is still open after 10 minutes.\n# TYPE edu_math_jobs_unsettled_reservations gauge\nedu_math_jobs_unsettled_reservations %d\n", unsettled)
	fmt.Fprintf(&b, "# HELP edu_ledger_anomalies_open Unresolved education ledger anomalies.\n# TYPE edu_ledger_anomalies_open gauge\nedu_ledger_anomalies_open %d\n", anomalies)
	fmt.Fprintf(&b, "# HELP edu_math_jobs_finished_window Finished (succeeded+failed) jobs in the last 24 hours.\n# TYPE edu_math_jobs_finished_window gauge\nedu_math_jobs_finished_window %d\n", terminal)
	fmt.Fprintf(&b, "# HELP edu_math_jobs_failure_ratio Failed share of finished jobs in the last 24 hours (0 if fewer than %d finished).\n# TYPE edu_math_jobs_failure_ratio gauge\nedu_math_jobs_failure_ratio %.4f\n", failureRateMinJobs, ratio)
	return b.String()
}
