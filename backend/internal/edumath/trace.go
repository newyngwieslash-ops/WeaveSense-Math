package edumath

import (
	"context"
	"regexp"

	"github.com/gin-gonic/gin"

	"yingce/backend/internal/flow"
)

// Request trace ids (DEV-121). Every /api/edu response carries X-Request-Id: the
// caller's own id when it is a plain token (8 to 64 of A-Z a-z 0-9 . _ -),
// otherwise a new one. The id rides on the request context, is stored on jobs
// accepted by that request, and appears in the job trace, so a support ticket
// that quotes the header leads to the job, its stages, settlement and revision.

const requestIDHeader = "X-Request-Id"

var requestIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

type requestIDKey struct{}

func withRequestID(c *gin.Context) string {
	rid := c.GetHeader(requestIDHeader)
	if !requestIDRe.MatchString(rid) {
		rid = flow.NewID("req_")
	}
	c.Header(requestIDHeader, rid)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDKey{}, rid))
	return rid
}

// requestIDFrom returns the request id on ctx, or "" outside an HTTP request.
func requestIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(requestIDKey{}).(string)
	return v
}
