package edumath

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestErrIfStandinDoc(t *testing.T) {
	ok := []string{
		``,
		`{}`,
		`{"meta":{}}`,
		`{"meta":{"ai_generated":true,"ai_parts":["narration","audio"]}}`,
		`{"meta":{"media_standin":null}}`,
		`{"meta":{"media_standin":{}}}`,
		`not json`,
	}
	for _, doc := range ok {
		if err := errIfStandinDoc([]byte(doc)); err != nil {
			t.Errorf("accepted lesson rejected: %s (%v)", doc, err)
		}
	}
	bad := []string{
		`{"meta":{"standin":true}}`,
		`{"meta":{"media_standin":{"provider":"fake-standin"}}}`,
		`{"meta":{"provider":"fake-standin"}}`,
		`{"meta":{"ai_parts":["narration","audio","standin"]}}`,
		`{"meta":{"standin":true,"ai_parts":["standin"],"media_standin":{"provider":"fake-standin"}}}`,
	}
	for _, doc := range bad {
		err := errIfStandinDoc([]byte(doc))
		if err == nil {
			t.Errorf("stand-in lesson accepted: %s", doc)
			continue
		}
		ae, ok := err.(*AppError)
		if !ok || ae.Code != "STANDIN_MEDIA" || ae.Status != 409 {
			t.Errorf("%s: got %v", doc, err)
		}
	}
}

func TestProvenanceMarksStandin(t *testing.T) {
	p := provenanceOf([]byte(`{"meta":{"ai_generated":true,"generator":"fake-standin","ai_parts":["narration","audio","standin"],"standin":true,"media_standin":{"provider":"fake-standin"}}}`))
	if !p.Standin || p.Kind != "model_assisted" {
		t.Fatalf("standin=%v kind=%s", p.Standin, p.Kind)
	}
	if !strings.Contains(p.Label, "本地替身") || !strings.Contains(p.Label, "不可作为正式课件发布") {
		t.Fatalf("label %q", p.Label)
	}
	if provenanceOf([]byte(`{"meta":{"ai_generated":false,"generator":"deterministic-template"}}`)).Standin {
		t.Fatal("plain template marked stand-in")
	}
}

func TestProblemImageOCRStandinOff(t *testing.T) {
	h := newHarness(t, startMathService(t)) // MATH_MEDIA_STANDIN unset, default off
	_ = h.setupTeacher()
	src := h.mustSource("u_alice")
	var buf bytes.Buffer
	if err := png.Encode(&buf, testPicture(32, 24)); err != nil {
		t.Fatal(err)
	}
	c, out, raw := h.uploadImage("u_alice", buf.Bytes(), "q.png", "image/png", src)
	if c != 201 {
		t.Fatalf("upload: %d %s", c, raw)
	}
	id := data(out)["id"].(string)
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/problem-images/"+id+"/ocr", nil)
	if c == 200 {
		t.Fatalf("OCR succeeded with stand-in off: %s", raw)
	}
	if code := errCode(out); code != "PROVIDER_UNAVAILABLE" {
		t.Fatalf("ocr with stand-in off: %d code=%s %s", c, code, raw)
	}
}
