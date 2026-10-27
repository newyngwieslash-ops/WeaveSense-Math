package edumath

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

func TestProblemImages(t *testing.T) { runProblemImages(t, newHarness(t, startMathService(t))) }

func TestPGProblemImages(t *testing.T) {
	runProblemImages(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

// uploadImage posts one picture as multipart; ct and name are the declared type and file name.
func (h *harness) uploadImage(user string, raw []byte, name, ct, sourceID string, hdr ...string) (int, map[string]any, []byte) {
	h.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("sourceAssetId", sourceID)
	ph := textproto.MIMEHeader{}
	ph.Set("Content-Disposition", `form-data; name="file"; filename="`+name+`"`)
	if ct != "" {
		ph.Set("Content-Type", ct)
	}
	w, _ := mw.CreatePart(ph)
	_, _ = w.Write(raw)
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/api/edu/problem-images", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.r.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.Bytes()
}

func testPicture(w, hh int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, hh))
	for y := 0; y < hh; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8(x + y), 255})
		}
	}
	return img
}

func pngChunk(typ string, body []byte) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, uint32(len(body)))
	b.WriteString(typ)
	b.Write(body)
	_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), body...)))
	return b.Bytes()
}

// pngWithText is a valid PNG carrying a tEXt chunk right after IHDR.
func pngWithText(w, hh int, text string) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, testPicture(w, hh))
	raw := b.Bytes()
	ihdrEnd := 8 + 8 + 13 + 4
	out := append([]byte{}, raw[:ihdrEnd]...)
	out = append(out, pngChunk("tEXt", []byte("Author\x00"+text))...)
	return append(out, raw[ihdrEnd:]...)
}

// pngHeaderOnly claims a canvas of w x h without pixel data (a decompression bomb header).
func pngHeaderOnly(w, hh uint32) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], hh)
	ihdr[8], ihdr[9] = 8, 2
	out := []byte("\x89PNG\r\n\x1a\n")
	out = append(out, pngChunk("IHDR", ihdr)...)
	out = append(out, pngChunk("IDAT", []byte{0x78, 0x9c, 0x03, 0x00, 0x00, 0x00, 0x00, 0x01})...)
	return append(out, pngChunk("IEND", nil)...)
}

// jpegWithExif is a valid JPEG carrying an APP1 Exif segment with a GPS marker.
func jpegWithExif(w, hh int, marker string) []byte {
	var b bytes.Buffer
	_ = jpeg.Encode(&b, testPicture(w, hh), &jpeg.Options{Quality: 85})
	raw := b.Bytes()
	payload := append([]byte("Exif\x00\x00GPSInfo "), []byte(marker)...)
	seg := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	seg = append(seg, payload...)
	out := append([]byte{}, raw[:2]...)
	out = append(out, seg...)
	return append(out, raw[2:]...)
}

func runProblemImages(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	pubID, _ := publishG1(t, h)
	var pub model.EduPublication
	h.db.First(&pub, "id = ?", pubID)
	var lesson model.EduLessonRevision
	h.db.First(&lesson, "id = ?", pub.LessonRevisionID)
	var prev model.EduProblemRevision
	h.db.First(&prev, "tenant_id = ?", tenant)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/source-assets", map[string]any{"title": "自编题", "kind": "teacher_original", "allowStudent": true})
	src := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/source-assets", map[string]any{"title": "教辅", "kind": "publisher", "attribution": "某出版社"})
	src2 := data(out)["id"].(string)

	// Metadata is removed: PNG text chunk and JPEG Exif with GPS never reach storage.
	personal := "张三 13800000000"
	rawPNG := pngWithText(320, 200, personal)
	c, out, raw := h.uploadImage("u_alice", rawPNG, "../../etc/passwd.png", "image/png", src)
	if c != 201 {
		t.Fatalf("upload png: %d %s", c, raw)
	}
	orig := data(out)
	origID := orig["id"].(string)
	if orig["kind"] != "original" || orig["width"].(float64) != 320 || orig["height"].(float64) != 200 || orig["uploadSha256"] != sha(rawPNG) || strings.Contains(string(raw), "passwd") {
		t.Fatalf("upload record: %s", raw)
	}
	c, _, body, hdr := h.call("u_alice", "GET", "/api/edu/problem-images/"+origID+"/content", nil)
	if c != 200 || hdr.Get("Content-Type") != "image/png" || hdr.Get("Cache-Control") != "no-store" || hdr.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(hdr.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("content: %d %v", c, hdr)
	}
	if bytes.Contains(body, []byte("tEXt")) || bytes.Contains(body, []byte("13800000000")) || sha(body) != orig["sha256"] {
		t.Fatal("stored picture still carries the text chunk or does not match its hash")
	}
	if cfg, err := png.DecodeConfig(bytes.NewReader(body)); err != nil || cfg.Width != 320 {
		t.Fatalf("stored png: %v", err)
	}
	rawJPG := jpegWithExif(200, 120, "31.2304N 121.4737E")
	c, out, raw = h.uploadImage("u_alice", rawJPG, "photo.JPG", "image/jpeg", src)
	if c != 201 || data(out)["mime"] != "image/jpeg" {
		t.Fatalf("upload jpeg: %d %s", c, raw)
	}
	_, _, body, _ = h.call("u_alice", "GET", "/api/edu/problem-images/"+data(out)["id"].(string)+"/content", nil)
	if bytes.Contains(body, []byte("Exif")) || bytes.Contains(body, []byte("GPSInfo")) || bytes.Contains(body, []byte("121.4737")) {
		t.Fatal("stored jpeg still carries Exif or GPS data")
	}

	// Hostile and out of range uploads (DEV-109): the server checks content, not names.
	big := append(append([]byte{}, rawPNG...), make([]byte, maxProblemImageBytes)...)
	cases := []struct {
		name, file, ct string
		raw            []byte
		status         int
		code           string
	}{
		{"over 10 MB", "a.png", "image/png", big, 413, "IMAGE_TOO_LARGE"},
		{"svg named png", "a.png", "image/png", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), 415, "UNSUPPORTED_IMAGE"},
		{"gif declared", "a.gif", "image/gif", []byte("GIF89a\x01\x00\x01\x00"), 415, "UNSUPPORTED_IMAGE"},
		{"png named jpg", "a.jpg", "image/jpeg", rawPNG, 415, "IMAGE_TYPE_MISMATCH"},
		{"png declared jpeg", "a.png", "image/jpeg", rawPNG, 415, "IMAGE_TYPE_MISMATCH"},
		{"exe named png", "a.png", "", append([]byte("MZ\x90\x00"), make([]byte, 64)...), 415, "UNSUPPORTED_IMAGE"},
		{"png magic then garbage", "a.png", "image/png", append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{7}, 200)...), 422, "IMAGE_DECODE_FAILED"},
		{"bomb header 100000x100000", "a.png", "image/png", pngHeaderOnly(100000, 100000), 422, "IMAGE_DIMENSIONS"},
		{"edge 8193", "a.png", "image/png", pngHeaderOnly(8193, 16), 422, "IMAGE_DIMENSIONS"},
		{"8192x8192 over pixel budget", "a.png", "image/png", pngHeaderOnly(8192, 8192), 422, "IMAGE_DIMENSIONS"},
		{"too small", "a.png", "image/png", pngWithText(8, 8, "x"), 422, "IMAGE_DIMENSIONS"},
	}
	for _, tc := range cases {
		c, out, raw := h.uploadImage("u_alice", tc.raw, tc.file, tc.ct, src)
		if c != tc.status || errCode(out) != tc.code {
			t.Errorf("%s: want %d %s, got %d %s", tc.name, tc.status, tc.code, c, raw)
		}
	}
	if c, _, raw := h.uploadImage("u_alice", rawPNG, "a.png", "image/png", ""); c != 400 {
		t.Fatalf("missing source: %d %s", c, raw)
	}

	// Rights: a withdrawn record refuses new uploads; another tenant's record does not exist.
	h.call("u_alice", "POST", "/api/edu/source-assets/"+src2+"/withdraw", map[string]any{"reason": "授权到期", "confirm": true})
	var s2 model.EduSourceAsset
	h.db.First(&s2, "id = ?", src2)
	if s2.State == "withdrawn" {
		if c, out, raw := h.uploadImage("u_alice", rawPNG, "a.png", "image/png", src2); c != 409 || errCode(out) != "RIGHTS_RESTRICTED" {
			t.Fatalf("withdrawn rights: %d %s", c, raw)
		}
	} else {
		t.Fatalf("source not withdrawn: %+v", s2)
	}
	for _, r := range []string{"teacher"} {
		h.call("u_carol", "POST", "/api/edu/roles", map[string]any{"tenantId": model.PersonalTenantID("u_carol"), "userId": "u_carol", "role": r})
	}
	if c, _, raw := h.uploadImage("u_carol", rawPNG, "a.png", "image/png", src); c != 404 {
		t.Fatalf("other tenant with victim source: %d %s", c, raw)
	}
	for _, p := range []string{"/api/edu/problem-images/" + origID, "/api/edu/problem-images/" + origID + "/content"} {
		if c, _, raw, _ := h.call("u_carol", "GET", p, nil); c != 404 && c != 403 {
			t.Fatalf("other tenant %s: %d %s", p, c, raw)
		}
	}

	// A student never reaches problem pictures.
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高二(7)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "题图", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-img"})
	stu := []string{"X-Edu-Tenant", tenant}
	for _, p := range []string{"/api/edu/problem-images", "/api/edu/problem-images/" + origID + "/content"} {
		if c, _, raw, _ := h.call("u_bob", "GET", p, nil, stu...); c != 403 {
			t.Fatalf("student %s: %d %s", p, c, raw)
		}
	}
	if c, _, raw := h.uploadImage("u_bob", rawPNG, "a.png", "image/png", src, stu...); c != 403 {
		t.Fatalf("student upload: %d %s", c, raw)
	}

	// Select one problem: the original cannot be attached; the selection must lie inside.
	pi := "/api/edu/problem-images/"
	if c, out, raw, _ := h.call("u_alice", "PUT", "/api/edu/problems/"+prev.ProblemID+"/image", map[string]any{"imageId": origID}); c != 409 || errCode(out) != "IMAGE_NOT_SINGLE" {
		t.Fatalf("attach original: %d %s", c, raw)
	}
	for _, bad := range []map[string]any{{"x": 300, "y": 0, "w": 40, "h": 40}, {"x": -1, "y": 0, "w": 40, "h": 40}, {"x": 0, "y": 0, "w": 8, "h": 40}} {
		if c, _, raw, _ := h.call("u_alice", "POST", pi+origID+"/select", bad); c != 400 {
			t.Fatalf("bad selection %v: %d %s", bad, c, raw)
		}
	}
	c, out, raw, _ = h.call("u_alice", "POST", pi+origID+"/select", map[string]any{"x": 20, "y": 10, "w": 160, "h": 90})
	if c != 201 || data(out)["kind"] != "single" || data(out)["parentId"] != origID || data(out)["width"].(float64) != 160 || data(out)["sourceAssetId"] != src {
		t.Fatalf("select: %d %s", c, raw)
	}
	single := data(out)["id"].(string)
	_, _, body, _ = h.call("u_alice", "GET", pi+single+"/content", nil)
	cropped, err := png.Decode(bytes.NewReader(body))
	if err != nil || cropped.Bounds().Dx() != 160 || cropped.Bounds().Dy() != 90 {
		t.Fatalf("cropped picture: %v", err)
	}
	if r, g, _, _ := cropped.At(0, 0).RGBA(); r>>8 != 20 || g>>8 != 10 {
		t.Fatalf("crop origin wrong: %d %d", r>>8, g>>8)
	}
	c, out, raw, _ = h.call("u_alice", "POST", pi+origID+"/select", map[string]any{"whole": true})
	if c != 201 || data(out)["width"].(float64) != 320 {
		t.Fatalf("whole: %d %s", c, raw)
	}
	whole := data(out)["id"].(string)

	// Attach: links the problem to the picture's rights record; a different record is refused.
	c, out, raw, _ = h.call("u_alice", "PUT", "/api/edu/problems/"+prev.ProblemID+"/image", map[string]any{"imageId": single})
	if c != 200 || data(out)["problemId"] != prev.ProblemID {
		t.Fatalf("attach: %d %s", c, raw)
	}
	var p model.EduProblem
	h.db.First(&p, "id = ?", prev.ProblemID)
	if p.SourceAssetID != src {
		t.Fatalf("problem source not linked: %q", p.SourceAssetID)
	}
	c, out, raw, _ = h.call("u_alice", "GET", "/api/edu/problem-images?problemId="+prev.ProblemID, nil)
	if imgs, _ := data(out)["images"].([]any); c != 200 || len(imgs) != 1 || imgs[0].(map[string]any)["id"] != single || strings.Contains(string(raw), `"content"`) {
		t.Fatalf("list by problem: %d %v", c, out)
	}
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/source-assets", map[string]any{"title": "另一份", "kind": "teacher_original"})
	src3 := data(out)["id"].(string)
	_, out, _ = h.uploadImage("u_alice", rawPNG, "b.png", "image/png", src3)
	_, out, _, _ = h.call("u_alice", "POST", pi+data(out)["id"].(string)+"/select", map[string]any{"whole": true})
	if c, out, raw, _ := h.call("u_alice", "PUT", "/api/edu/problems/"+prev.ProblemID+"/image", map[string]any{"imageId": data(out)["id"]}); c != 409 || errCode(out) != "RIGHTS_RESTRICTED" {
		t.Fatalf("attach under other rights: %d %s", c, raw)
	}

	// The student's lesson never names the picture.
	if c, _, raw, _ := h.call("u_bob", "GET", "/api/edu/student/assignments/"+asID, nil, stu...); c != 200 || strings.Contains(string(raw), single) || strings.Contains(string(raw), "epi_") {
		t.Fatalf("student view: %d %s", c, raw)
	}

	// Withdraw drops the bytes; the content route then refuses.
	c, out, raw, _ = h.call("u_alice", "POST", pi+whole+"/withdraw", nil)
	if c != 200 || data(out)["changed"] != true {
		t.Fatalf("withdraw: %d %s", c, raw)
	}
	if c, _, _, _ := h.call("u_alice", "GET", pi+whole+"/content", nil); c != 404 {
		t.Fatalf("withdrawn content: %d", c)
	}
	var w model.EduProblemImage
	h.db.First(&w, "id = ?", whole)
	if len(w.Content) != 0 {
		t.Fatal("withdrawn picture kept its bytes")
	}

	// Retention: unattached pictures past the temporary period are deleted, the attached one stays.
	pol := model.EduRetentionPolicy{TempDays: 7, ImageDays: 90, LearningDays: 180, BackupDays: 30}
	later := time.Now().UTC().AddDate(0, 0, 8)
	sum, err := applyRetention(h.db, tenant, pol, later, true)
	if err != nil || sum.ProblemImages != 5 {
		t.Fatalf("retention preview: %+v %v", sum, err)
	}
	if sum, err = applyRetention(h.db, tenant, pol, later, false); err != nil || sum.ProblemImages != 5 {
		t.Fatalf("retention run: %+v %v", sum, err)
	}
	var left []model.EduProblemImage
	h.db.Where("tenant_id = ?", tenant).Find(&left)
	if len(left) != 1 || left[0].ID != single {
		t.Fatalf("after retention: %d left", len(left))
	}

	var n int64
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action LIKE 'problem_image.%'", tenant).Count(&n)
	if n != 8 { // 3 uploads, 3 selections, 1 attach, 1 withdraw
		t.Fatalf("audits: %d", n)
	}
}
