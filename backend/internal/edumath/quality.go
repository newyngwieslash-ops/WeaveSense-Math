package edumath

import (
	"encoding/json"

	"gorm.io/gorm"

	"yingce/backend/internal/model"
)

// Teaching quality panel (DEV-075): math, teaching and presentation evidence are reported as separate
// dimensions and never merged into one score. Math comes from the independent verification of the solution
// revision; teaching is only the human review bound to this exact lesson hash; presentation is the clean
// re-render check (DEV-082). Audio is not generated yet, and the panel never plays media by itself.

type qualityMath struct {
	State          string   `json:"state"`
	SolutionStatus string   `json:"solutionStatus"`
	EvidenceLevels []string `json:"evidenceLevels"`
	DomainProof    bool     `json:"domainProof"`
	SampledOnly    bool     `json:"sampledOnly"`
	Source         string   `json:"source"`
}

type qualityTeaching struct {
	State         string         `json:"state"` // not_reviewed | approved | rejected | inherited
	HumanOnly     bool           `json:"humanOnly"`
	BoundHash     string         `json:"boundHash,omitempty"`
	SelfReview    bool           `json:"selfReview"`
	Checklist     map[string]int `json:"checklist"`
	Findings      int            `json:"findings"`
	Edited        bool           `json:"edited"`
	EditClasses   string         `json:"editClasses,omitempty"`
	StaleReviews  int            `json:"staleReviews"`
	ReviewRecords int            `json:"reviewRecords"`
}

type qualityPresentation struct {
	State            string   `json:"state"` // passed | warnings | failed | not_run
	Blocking         []string `json:"blocking"`
	Warnings         []string `json:"warnings"`
	MathVerification bool     `json:"mathVerification"`
}

type qualityAudio struct {
	State    string `json:"state"`
	Autoplay bool   `json:"autoplay"`
}

type QualityPanel struct {
	Math         qualityMath         `json:"math"`
	Teaching     qualityTeaching     `json:"teaching"`
	Presentation qualityPresentation `json:"presentation"`
	Audio        qualityAudio        `json:"audio"`
	Combined     bool                `json:"combinedScore"`
}

var domainProofLevels = map[string]bool{"symbolic_domain_identity": true, "symbolic_domain": true, "parametric_identity": true}

func qualityView(tx *gorm.DB, l model.EduLessonRevision, reviews []model.EduReviewRecord) QualityPanel {
	q := QualityPanel{Audio: qualityAudio{State: "not_generated", Autoplay: false}}

	q.Math = qualityMath{State: "unknown", EvidenceLevels: []string{}, Source: "math_service_verify"}
	var sol model.EduSolutionRevision
	if tx.First(&sol, "id = ? AND tenant_id = ?", l.SolutionRevisionID, l.TenantID).Error == nil {
		q.Math.State, q.Math.SolutionStatus = sol.MathState, sol.SolutionStatus
		_ = json.Unmarshal([]byte(sol.EvidenceLevels), &q.Math.EvidenceLevels)
		if q.Math.EvidenceLevels == nil {
			q.Math.EvidenceLevels = []string{}
		}
		sampled := false
		for _, lv := range q.Math.EvidenceLevels {
			if domainProofLevels[lv] {
				q.Math.DomainProof = true
			}
			if lv == "numeric_sampling" {
				sampled = true
			}
		}
		q.Math.SampledOnly = sampled && !q.Math.DomainProof
		if sol.MathState == ManualUnverified {
			q.Math.Source = "teacher_manual" // DEV-039: written by the teacher; no automatic math evidence
		}
	}

	t := qualityTeaching{State: "not_reviewed", HumanOnly: true, Checklist: map[string]int{"pass": 0, "issue": 0, "na": 0},
		Edited: l.ParentLessonRevisionID != "", EditClasses: l.EditClasses, ReviewRecords: len(reviews)}
	for _, r := range reviews {
		if r.LessonHash != l.LessonHash {
			t.StaleReviews++
			continue
		}
		// The latest record bound to this exact hash decides the teaching state.
		t.BoundHash, t.SelfReview = r.LessonHash, r.SelfReview
		t.State = r.Decision
		if r.InheritedFrom != "" {
			t.State = "inherited"
		}
		t.Checklist = map[string]int{"pass": 0, "issue": 0, "na": 0}
		var ck map[string]string
		if json.Unmarshal([]byte(r.Checklist), &ck) == nil {
			for _, v := range ck {
				if _, ok := t.Checklist[v]; ok {
					t.Checklist[v]++
				}
			}
		}
		var fs []json.RawMessage
		t.Findings = 0
		if json.Unmarshal([]byte(r.Findings), &fs) == nil {
			t.Findings = len(fs)
		}
	}
	q.Teaching = t

	q.Presentation = qualityPresentation{State: "not_run", Blocking: []string{}, Warnings: []string{}}
	if rc := renderCheckView(l); rc != nil {
		q.Presentation.State = rc.State
		q.Presentation.Blocking = nonNil(rc.Blocking)
		q.Presentation.Warnings = nonNil(rc.Warnings)
	}
	return q
}
