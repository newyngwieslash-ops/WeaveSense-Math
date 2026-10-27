package edumath

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// DEV-068 single problem upload and crop. A teacher uploads one JPG or PNG
// (at most 10 MB, longest edge at most 8192 px) under a rights record. The
// server trusts neither the file name nor the declared type: it reads the
// magic bytes, reads the header dimensions before decoding (so a small file
// that claims a huge canvas is refused without allocating it), decodes, and
// stores only a re-encoded copy, which carries no EXIF, GPS, ICC or text
// chunks. A picture may hold several problems, so it can only be attached to
// a problem after the teacher selects one region (or confirms the whole
// picture holds one problem), which produces a new single image. OCR is not
// done here (DEV-069 needs model credentials): the teacher types the
// statement next to the picture. Problem images are teacher material and are
// never served to students.

const (
	maxProblemImageBytes  = 10 << 20
	maxProblemImageEdge   = 8192
	maxProblemImagePixels = 40_000_000 // decode budget: about 160 MB as RGBA
	minProblemImageEdge   = 16
	maxProblemImageField  = 200
)

// decodeSlots bounds concurrent full decodes so uploads cannot exhaust memory.
var decodeSlots = make(chan struct{}, 2)

var errImageBusy = appErr(503, "IMAGE_BUSY", "too many images are being processed, retry shortly", "retry_later")

func imageErr(status int, code, msg string) *AppError {
	return appErr(status, code, msg, "choose_another_image")
}

func (h *API) registerProblemImages(g *gin.RouterGroup) {
	g.POST("/problem-images", h.uploadProblemImage)
	g.GET("/problem-images", h.listProblemImages)
	g.GET("/problem-images/:id", h.getProblemImage)
	g.GET("/problem-images/:id/content", h.problemImageContent)
	g.POST("/problem-images/:id/select", h.selectProblemImage)
	g.POST("/problem-images/:id/withdraw", h.withdrawProblemImage)
	g.PUT("/problems/:id/image", h.attachProblemImage)
}

func (h *API) imageTeacher(c *gin.Context) (Actor, *gorm.DB, bool) {
	a, tx, ok := h.staff(c)
	if !ok {
		return a, tx, false
	}
	if !a.Has("teacher") {
		h.fail(c, ErrNoRole)
		return a, tx, false
	}
	return a, tx, true
}

// sniffImage returns "image/png" or "image/jpeg" from the magic bytes only.
func sniffImage(b []byte) string {
	switch {
	case len(b) >= 8 && bytes.Equal(b[:8], []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "image/jpeg"
	}
	return ""
}

// declaredImageType maps the declared content type and file extension to a
// type; "" means nothing was declared, "?" means something unsupported was.
func declaredImageType(contentType, filename string) (string, string) {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch ct {
	case "", "application/octet-stream":
		ct = ""
	case "image/png":
	case "image/jpeg", "image/jpg", "image/pjpeg":
		ct = "image/jpeg"
	default:
		ct = "?"
	}
	ext := strings.ToLower(path.Ext(strings.ReplaceAll(filename, "\\", "/")))
	switch ext {
	case "":
	case ".png":
		ext = "image/png"
	case ".jpg", ".jpeg", ".jpe":
		ext = "image/jpeg"
	default:
		ext = "?"
	}
	return ct, ext
}

// cleanImage checks and re-encodes raw bytes. It returns the cleaned bytes,
// mime and dimensions, or an *AppError.
func cleanImage(raw []byte) ([]byte, string, int, int, error) {
	mime := sniffImage(raw)
	if mime == "" {
		return nil, "", 0, 0, imageErr(415, "UNSUPPORTED_IMAGE", "only JPG and PNG pictures are accepted")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format == "png") != (mime == "image/png") {
		return nil, "", 0, 0, imageErr(422, "IMAGE_DECODE_FAILED", "the picture cannot be read")
	}
	if cfg.Width < minProblemImageEdge || cfg.Height < minProblemImageEdge || cfg.Width > maxProblemImageEdge || cfg.Height > maxProblemImageEdge ||
		int64(cfg.Width)*int64(cfg.Height) > maxProblemImagePixels {
		return nil, "", 0, 0, imageErr(422, "IMAGE_DIMENSIONS", fmt.Sprintf("each edge 16..%d px and at most %d pixels", maxProblemImageEdge, maxProblemImagePixels))
	}
	select {
	case decodeSlots <- struct{}{}:
		defer func() { <-decodeSlots }()
	default:
		return nil, "", 0, 0, errImageBusy
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", 0, 0, imageErr(422, "IMAGE_DECODE_FAILED", "the picture cannot be read")
	}
	b := img.Bounds()
	out, err := encodeImage(img, mime)
	if err != nil {
		return nil, "", 0, 0, err
	}
	return out, mime, b.Dx(), b.Dy(), nil
}

// encodeImage writes pixels only; the standard encoders emit no metadata.
func encodeImage(img image.Image, mime string) ([]byte, error) {
	var buf bytes.Buffer
	var err error
	if mime == "image/png" {
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img)
	} else {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	}
	return buf.Bytes(), err
}

func sha(b []byte) string { s := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(s[:]) }

// readImageUpload reads the multipart form: one "file" part and "sourceAssetId".
func readImageUpload(c *gin.Context) (raw []byte, ct, filename, sourceID string, err error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxProblemImageBytes+(64<<10))
	mr, err := c.Request.MultipartReader()
	if err != nil {
		return nil, "", "", "", appErr(400, "VALIDATION_FAILED", "multipart form with file and sourceAssetId required", "fix_input")
	}
	for {
		part, perr := mr.NextPart()
		if perr == io.EOF {
			break
		}
		if perr != nil {
			var mbe *http.MaxBytesError
			if errors.As(perr, &mbe) {
				return nil, "", "", "", imageErr(413, "IMAGE_TOO_LARGE", "a picture may be at most 10 MB")
			}
			return nil, "", "", "", appErr(400, "VALIDATION_FAILED", "malformed multipart form", "fix_input")
		}
		switch part.FormName() {
		case "file":
			if raw != nil {
				return nil, "", "", "", appErr(400, "VALIDATION_FAILED", "exactly one picture per upload", "fix_input")
			}
			raw, err = io.ReadAll(io.LimitReader(part, maxProblemImageBytes+1))
			if err != nil {
				var mbe *http.MaxBytesError
				if errors.As(err, &mbe) {
					return nil, "", "", "", imageErr(413, "IMAGE_TOO_LARGE", "a picture may be at most 10 MB")
				}
				return nil, "", "", "", appErr(400, "VALIDATION_FAILED", "upload interrupted", "fix_input")
			}
			if len(raw) > maxProblemImageBytes {
				return nil, "", "", "", imageErr(413, "IMAGE_TOO_LARGE", "a picture may be at most 10 MB")
			}
			ct, filename = part.Header.Get("Content-Type"), part.FileName()
		case "sourceAssetId":
			v, _ := io.ReadAll(io.LimitReader(part, maxProblemImageField+1))
			sourceID = strings.TrimSpace(string(v))
		}
		part.Close()
	}
	if raw == nil || sourceID == "" || len(sourceID) > 40 {
		return nil, "", "", "", appErr(400, "VALIDATION_FAILED", "file and sourceAssetId required", "fix_input")
	}
	return raw, ct, filename, sourceID, nil
}

func (h *API) liveSource(tx *gorm.DB, a Actor, id string) (model.EduSourceAsset, error) {
	var src model.EduSourceAsset
	if tx.First(&src, "id = ? AND tenant_id = ? AND workspace_id = ?", id, a.TenantID, a.WorkspaceID).Error != nil {
		return src, ErrNotFound
	}
	if !rightsLive(src, h.Svc.now()) {
		return src, appErr(409, "RIGHTS_RESTRICTED", "the source rights record is withdrawn or expired", "check_source_rights")
	}
	return src, nil
}

func (h *API) uploadProblemImage(c *gin.Context) {
	a, tx, ok := h.imageTeacher(c)
	if !ok {
		return
	}
	raw, ct, filename, sourceID, err := readImageUpload(c)
	if err != nil {
		h.fail(c, err)
		return
	}
	src, err := h.liveSource(tx, a, sourceID)
	if err != nil {
		h.fail(c, err)
		return
	}
	sniffed := sniffImage(raw)
	dct, dext := declaredImageType(ct, filename)
	if dct == "?" || dext == "?" {
		h.fail(c, imageErr(415, "UNSUPPORTED_IMAGE", "only JPG and PNG pictures are accepted"))
		return
	}
	if sniffed != "" && ((dct != "" && dct != sniffed) || (dext != "" && dext != sniffed)) {
		h.fail(c, imageErr(415, "IMAGE_TYPE_MISMATCH", "the file content does not match its name or declared type"))
		return
	}
	clean, mime, w, hh, err := cleanImage(raw)
	if err != nil {
		h.fail(c, err)
		return
	}
	now := h.Svc.now()
	img := model.EduProblemImage{ID: flow.NewID("epi_"), TenantID: a.TenantID, WorkspaceID: a.WorkspaceID, Kind: "original", State: "active",
		SourceAssetID: src.ID, Mime: mime, Width: w, Height: hh, Bytes: len(clean), SHA256: sha(clean), UploadSHA256: sha(raw),
		CropW: w, CropH: hh, Content: clean, CreatedBy: a.UserID, CreatedAt: now, UpdatedAt: now}
	if err := tx.Create(&img).Error; err != nil {
		h.fail(c, err)
		return
	}
	h.Svc.audit(tx, a, "problem_image.upload", img.ID, src.ID+" "+img.UploadSHA256)
	reply(c, 201, img, "ok")
}

func (h *API) loadProblemImage(c *gin.Context, tx *gorm.DB, a Actor) (model.EduProblemImage, bool) {
	var img model.EduProblemImage
	if tx.Omit("content").First(&img, "id = ? AND tenant_id = ? AND workspace_id = ?", c.Param("id"), a.TenantID, a.WorkspaceID).Error != nil {
		h.fail(c, ErrNotFound)
		return img, false
	}
	return img, true
}

func (h *API) listProblemImages(c *gin.Context) {
	a, tx, ok := h.imageTeacher(c)
	if !ok {
		return
	}
	q := tx.Omit("content").Where("tenant_id = ? AND workspace_id = ? AND state = 'active'", a.TenantID, a.WorkspaceID)
	if p := c.Query("problemId"); p != "" {
		q = q.Where("problem_id = ?", p)
	}
	var out []model.EduProblemImage
	if err := q.Order("created_at DESC").Limit(100).Find(&out).Error; err != nil {
		h.fail(c, err)
		return
	}
	reply(c, 200, gin.H{"images": out}, "ok")
}

func (h *API) getProblemImage(c *gin.Context) {
	a, tx, ok := h.imageTeacher(c)
	if !ok {
		return
	}
	if img, ok := h.loadProblemImage(c, tx, a); ok {
		reply(c, 200, img, "ok")
	}
}

func (h *API) problemImageContent(c *gin.Context) {
	a, tx, ok := h.imageTeacher(c)
	if !ok {
		return
	}
	var img model.EduProblemImage
	if tx.First(&img, "id = ? AND tenant_id = ? AND workspace_id = ? AND state = 'active'", c.Param("id"), a.TenantID, a.WorkspaceID).Error != nil {
		h.fail(c, ErrNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("Content-Disposition", "inline")
	c.Data(200, img.Mime, img.Content)
}

func (h *API) selectProblemImage(c *gin.Context) {
	a, tx, ok := h.imageTeacher(c)
	if !ok {
		return
	}
	var in struct {
		X, Y, W, H int
		Whole      bool `json:"whole"`
	}
	if c.ShouldBindJSON(&in) != nil {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "x, y, w, h in pixels, or whole", "fix_input"))
		return
	}
	var img model.EduProblemImage
	if tx.First(&img, "id = ? AND tenant_id = ? AND workspace_id = ? AND state = 'active' AND kind = 'original'", c.Param("id"), a.TenantID, a.WorkspaceID).Error != nil {
		h.fail(c, ErrNotFound)
		return
	}
	if in.Whole {
		in.X, in.Y, in.W, in.H = 0, 0, img.Width, img.Height
	}
	if in.X < 0 || in.Y < 0 || in.W < minProblemImageEdge || in.H < minProblemImageEdge || in.X+in.W > img.Width || in.Y+in.H > img.Height {
		h.fail(c, appErr(400, "VALIDATION_FAILED", fmt.Sprintf("selection must lie inside the %dx%d picture and be at least 16 px each way", img.Width, img.Height), "fix_input"))
		return
	}
	if _, err := h.liveSource(tx, a, img.SourceAssetID); err != nil {
		h.fail(c, err)
		return
	}
	select {
	case decodeSlots <- struct{}{}:
	default:
		h.fail(c, errImageBusy)
		return
	}
	pix, _, err := image.Decode(bytes.NewReader(img.Content))
	var out []byte
	if err == nil {
		r := image.Rect(in.X, in.Y, in.X+in.W, in.Y+in.H).Add(pix.Bounds().Min)
		dst := image.NewRGBA(image.Rect(0, 0, in.W, in.H))
		draw.Draw(dst, dst.Bounds(), pix, r.Min, draw.Src)
		out, err = encodeImage(dst, img.Mime)
	}
	<-decodeSlots
	if err != nil {
		h.fail(c, err)
		return
	}
	now := h.Svc.now()
	single := model.EduProblemImage{ID: flow.NewID("epi_"), TenantID: a.TenantID, WorkspaceID: a.WorkspaceID, Kind: "single", State: "active",
		ParentID: img.ID, SourceAssetID: img.SourceAssetID, Mime: img.Mime, Width: in.W, Height: in.H, Bytes: len(out), SHA256: sha(out),
		UploadSHA256: img.UploadSHA256, CropX: in.X, CropY: in.Y, CropW: in.W, CropH: in.H, Content: out, CreatedBy: a.UserID, CreatedAt: now, UpdatedAt: now}
	if err := tx.Create(&single).Error; err != nil {
		h.fail(c, err)
		return
	}
	h.Svc.audit(tx, a, "problem_image.select", single.ID, fmt.Sprintf("%s %d,%d,%d,%d", img.ID, in.X, in.Y, in.W, in.H))
	reply(c, 201, single, "ok")
}

func (h *API) attachProblemImage(c *gin.Context) {
	a, tx, ok := h.imageTeacher(c)
	if !ok {
		return
	}
	var in struct {
		ImageID string `json:"imageId"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ImageID == "" {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "imageId required", "fix_input"))
		return
	}
	var p model.EduProblem
	if tx.First(&p, "id = ? AND tenant_id = ? AND workspace_id = ?", c.Param("id"), a.TenantID, a.WorkspaceID).Error != nil {
		h.fail(c, ErrNotFound)
		return
	}
	var img model.EduProblemImage
	if tx.Omit("content").First(&img, "id = ? AND tenant_id = ? AND workspace_id = ? AND state = 'active'", in.ImageID, a.TenantID, a.WorkspaceID).Error != nil {
		h.fail(c, ErrNotFound)
		return
	}
	if img.Kind != "single" {
		h.fail(c, appErr(409, "IMAGE_NOT_SINGLE", "select the one problem on the picture first", "select_region"))
		return
	}
	if _, err := h.liveSource(tx, a, img.SourceAssetID); err != nil {
		h.fail(c, err)
		return
	}
	if p.SourceAssetID != "" && p.SourceAssetID != img.SourceAssetID {
		h.fail(c, appErr(409, "RIGHTS_RESTRICTED", "the picture was uploaded under a different rights record than the problem", "new_problem"))
		return
	}
	if p.SourceAssetID == "" {
		if err := tx.Model(&p).Update("source_asset_id", img.SourceAssetID).Error; err != nil {
			h.fail(c, err)
			return
		}
		h.Svc.audit(tx, a, "source.link", p.ID, img.SourceAssetID)
	}
	now := h.Svc.now()
	if err := tx.Model(&model.EduProblemImage{}).Where("tenant_id = ? AND problem_id = ? AND id <> ?", a.TenantID, p.ID, img.ID).
		Updates(map[string]any{"problem_id": "", "updated_at": now}).Error; err != nil {
		h.fail(c, err)
		return
	}
	if err := tx.Model(&img).Updates(map[string]any{"problem_id": p.ID, "updated_at": now}).Error; err != nil {
		h.fail(c, err)
		return
	}
	img.ProblemID = p.ID
	h.Svc.audit(tx, a, "problem_image.attach", p.ID, img.ID)
	reply(c, 200, img, "ok")
}

func (h *API) withdrawProblemImage(c *gin.Context) {
	a, tx, ok := h.imageTeacher(c)
	if !ok {
		return
	}
	img, ok := h.loadProblemImage(c, tx, a)
	if !ok {
		return
	}
	if img.State == "withdrawn" {
		reply(c, 200, gin.H{"image": img, "changed": false}, "ok")
		return
	}
	now := h.Svc.now()
	if err := tx.Model(&img).Updates(map[string]any{"state": "withdrawn", "content": []byte{}, "problem_id": "", "updated_at": now}).Error; err != nil {
		h.fail(c, err)
		return
	}
	img.State, img.ProblemID = "withdrawn", ""
	h.Svc.audit(tx, a, "problem_image.withdraw", img.ID, "")
	reply(c, 200, gin.H{"image": img, "changed": true}, "ok")
}
