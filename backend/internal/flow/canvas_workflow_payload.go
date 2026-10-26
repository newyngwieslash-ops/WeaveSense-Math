package flow

import (
 "bytes"
 "encoding/json"
 "errors"
 "net/url"
 "regexp"
 "strings"
 "unicode/utf8"
)

const CanvasWorkflowPayloadMaxBytes = 512 * 1024
const CanvasWorkflowPayloadMaxDepth = 32
const CanvasWorkflowPayloadMaxValues = 10000

var ErrCanvasWorkflowSensitivePayload = errors.New("flow: credential-bearing workflow payload is not allowed; use a server-side credential reference")
var ErrCanvasWorkflowPayloadLimit = errors.New("flow: workflow payload exceeds persistence limits")
var ErrCanvasWorkflowPayloadInvalid = errors.New("flow: workflow payload is not valid JSON")

var workflowURLPattern = regexp.MustCompile(`(?i)(?:https?|wss?|socks5)://[^\s<>"'\\]+`)
var workflowSecretPattern = regexp.MustCompile(`(?i)(?:\b(?:api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password|authorization|proxy[_-]?authorization|cookie|set[_-]?cookie)\b["']?\s*[:=]\s*[^\s"',}]+|\bbearer\s+[a-z0-9._~+/=-]+|\bsk-[a-z0-9_-]{20,}|-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----)`)

func workflowCredentialField(key string) bool {
 key = strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
 switch key {
 case "apikey", "xapikey", "xgoogapikey", "authorization", "proxyauthorization", "cookie", "setcookie", "password", "passwd", "secret", "clientsecret", "privatekey", "accesskey", "accesskeyid", "secretaccesskey", "awsaccesskeyid", "token", "accesstoken", "refreshtoken", "idtoken", "sessiontoken", "sessionid", "credential", "credentials", "signature", "sig", "xamzsignature", "xamzcredential", "xamzsecuritytoken", "xgoogsignature", "xgoogcredential":
  return true
 }
 return false
}

func workflowSensitiveURL(raw string) bool {
 u, err := url.Parse(raw)
 if err != nil { return true }
 if u.User != nil { return true }
 for _,part:=range []string{u.RawQuery,u.Fragment} {
  values,err:=url.ParseQuery(part)
  if err!=nil {return true}
  for key,vs:=range values {if workflowCredentialField(key) {for _,v:=range vs {if v!="" {return true}}}}
 }
 return false
}

// ValidateCanvasWorkflowPayload checks the JSON representation that will be
// persisted or sent to a model, including JSON embedded in string fields. It is
// a credential boundary, not a generic PII classifier: normal business text and
// source URLs remain unchanged. Opaque secrets must use server-side references.
// Errors deliberately contain neither payload values nor caller-controlled keys.
func ValidateCanvasWorkflowPayload(value any) error {
 raw,err:=json.Marshal(value)
 if err!=nil{return ErrCanvasWorkflowPayloadInvalid}
 if len(raw)>CanvasWorkflowPayloadMaxBytes{return ErrCanvasWorkflowPayloadLimit}
 decoder:=json.NewDecoder(bytes.NewReader(raw));decoder.UseNumber()
 var normalized any
 if decoder.Decode(&normalized)!=nil{return ErrCanvasWorkflowPayloadInvalid}
 count:=0
 var visit func(any,int)error
 visit=func(v any,depth int)error{
  count++
  if depth>CanvasWorkflowPayloadMaxDepth || count>CanvasWorkflowPayloadMaxValues{return ErrCanvasWorkflowPayloadLimit}
  switch v:=v.(type){
  case map[string]any:
   for key,item:=range v {
    if workflowCredentialField(key) && item!=nil {
     text,ok:=item.(string);if !ok || strings.TrimSpace(text)!="" {return ErrCanvasWorkflowSensitivePayload}
    }
    if err:=visit(key,depth+1);err!=nil{return err}
    if err:=visit(item,depth+1);err!=nil{return err}
   }
  case []any:
   for _,item:=range v{if err:=visit(item,depth+1);err!=nil{return err}}
  case string:
   if workflowSecretPattern.MatchString(v){return ErrCanvasWorkflowSensitivePayload}
   for _,rawURL:=range workflowURLPattern.FindAllString(v,-1){if workflowSensitiveURL(rawURL){return ErrCanvasWorkflowSensitivePayload}}
   text:=strings.TrimSpace(v)
   if strings.HasPrefix(text,"{") || strings.HasPrefix(text,"[") {
    var embedded any
    d:=json.NewDecoder(strings.NewReader(text));d.UseNumber()
    if json.Valid([]byte(text)) && d.Decode(&embedded)==nil {return visit(embedded,depth+1)}
   }
  }
  return nil
 }
 return visit(normalized,0)
}

// SanitizeCanvasWorkflowError preserves ordinary diagnostic text. Once a
// credential-shaped error is detected the complete error is replaced: keeping
// a guessed suffix could leak a multiline Cookie or quoted secret. It never
// logs the rejected text and limits persisted diagnostics to 1024 UTF-8 bytes.
func SanitizeCanvasWorkflowError(text string) string {
 if ValidateCanvasWorkflowPayload(text)!=nil {return "workflow error details omitted: unsafe or oversized diagnostic"}
 if len(text)<=1024{return text}
 end:=1024
 for end>0 && !utf8.RuneStart(text[end]) {end--}
 return text[:end]
}
