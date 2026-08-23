package phoenix

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	otlpContentType = "application/x-protobuf"
	projectName     = "svpchain-agent"
	projectHeader   = "x-project-name"
	projectAttr     = "openinference.project.name"
)

type otlpPayload struct {
	ResourceSpans []otlpResourceSpans
}

type otlpResourceSpans struct {
	Resource   otlpResource
	ScopeSpans []otlpScopeSpans
}

type otlpResource struct {
	Attributes []otlpAttr
}

type otlpScopeSpans struct {
	Scope otlpScope
	Spans []otlpSpan
}

type otlpScope struct {
	Name    string
	Version string
}

type otlpSpan struct {
	TraceID           string
	SpanID            string
	ParentSpanID      string
	Name              string
	Kind              int
	StartTimeUnixNano string
	EndTimeUnixNano   string
	Attributes        []otlpAttr
	Status            otlpStatus
}

type otlpStatus struct {
	Code    int
	Message string
}

type otlpAttr struct {
	Key   string
	Value otlpAttrValue
}

type otlpAttrValue struct {
	StringValue string
	IntValue    string
}

func strAttr(key, value string) otlpAttr {
	return otlpAttr{Key: key, Value: otlpAttrValue{StringValue: strings.TrimSpace(value)}}
}

func intAttr(key string, value int64) otlpAttr {
	return otlpAttr{Key: key, Value: otlpAttrValue{IntValue: fmt.Sprintf("%d", value)}}
}

func postOTLP(ctx context.Context, client *http.Client, endpoint string, payload otlpPayload) error {
	if client == nil || endpoint == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(marshalOTLP(payload)))
	if err != nil {
		return err
	}
	// Phoenix /v1/traces accepts only application/x-protobuf (JSON returns 415).
	req.Header.Set("Content-Type", otlpContentType)
	req.Header.Set(projectHeader, projectName)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("phoenix otlp: %s", resp.Status)
	}
	return nil
}

// marshalOTLP encodes ExportTraceServiceRequest (OTLP collector.trace.v1).
func marshalOTLP(payload otlpPayload) []byte {
	var b []byte
	for _, rs := range payload.ResourceSpans {
		b = appendMessage(b, 1, marshalResourceSpans(rs))
	}
	return b
}

func marshalResourceSpans(rs otlpResourceSpans) []byte {
	var b []byte
	b = appendMessage(b, 1, marshalResource(rs.Resource))
	for _, ss := range rs.ScopeSpans {
		b = appendMessage(b, 2, marshalScopeSpans(ss))
	}
	return b
}

func marshalResource(r otlpResource) []byte {
	var b []byte
	for _, a := range r.Attributes {
		b = appendMessage(b, 1, marshalAttr(a))
	}
	return b
}

func marshalScopeSpans(ss otlpScopeSpans) []byte {
	var b []byte
	b = appendMessage(b, 1, marshalScope(ss.Scope))
	for _, sp := range ss.Spans {
		b = appendMessage(b, 2, marshalSpan(sp))
	}
	return b
}

func marshalScope(s otlpScope) []byte {
	var b []byte
	b = appendString(b, 1, s.Name)
	b = appendString(b, 2, s.Version)
	return b
}

func marshalSpan(sp otlpSpan) []byte {
	var b []byte
	b = appendBytes(b, 1, idBytes(sp.TraceID))
	b = appendBytes(b, 2, idBytes(sp.SpanID))
	b = appendBytes(b, 4, idBytes(sp.ParentSpanID))
	b = appendString(b, 5, sp.Name)
	b = appendVarintField(b, 6, uint64(sp.Kind))
	b = appendFixed64(b, 7, parseNano(sp.StartTimeUnixNano))
	b = appendFixed64(b, 8, parseNano(sp.EndTimeUnixNano))
	for _, a := range sp.Attributes {
		b = appendMessage(b, 9, marshalAttr(a))
	}
	b = appendMessage(b, 15, marshalStatus(sp.Status))
	return b
}

func marshalStatus(s otlpStatus) []byte {
	var b []byte
	b = appendString(b, 2, s.Message)
	b = appendVarintField(b, 3, uint64(s.Code))
	return b
}

func marshalAttr(a otlpAttr) []byte {
	var b []byte
	b = appendString(b, 1, a.Key)
	b = appendMessage(b, 2, marshalAny(a.Value))
	return b
}

func marshalAny(v otlpAttrValue) []byte {
	var b []byte
	if v.IntValue != "" {
		n, _ := strconv.ParseInt(v.IntValue, 10, 64)
		return appendVarintField(b, 3, uint64(n))
	}
	return appendString(b, 1, v.StringValue)
}

func idBytes(hexID string) []byte {
	if hexID == "" {
		return nil
	}
	raw, err := hex.DecodeString(hexID)
	if err != nil {
		return nil
	}
	return raw
}

func parseNano(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n
}

func appendKey(b []byte, field, wire int) []byte {
	return appendVarint(b, uint64(field<<3|wire))
}

func appendVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func appendVarintField(b []byte, field int, v uint64) []byte {
	b = appendKey(b, field, 0)
	return appendVarint(b, v)
}

func appendFixed64(b []byte, field int, v uint64) []byte {
	b = appendKey(b, field, 1)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], v)
	return append(b, buf[:]...)
}

func appendBytes(b []byte, field int, data []byte) []byte {
	if len(data) == 0 {
		return b
	}
	b = appendKey(b, field, 2)
	b = appendVarint(b, uint64(len(data)))
	return append(b, data...)
}

func appendString(b []byte, field int, s string) []byte {
	if s == "" {
		return b
	}
	return appendBytes(b, field, []byte(s))
}

func appendMessage(b []byte, field int, msg []byte) []byte {
	return appendBytes(b, field, msg)
}
