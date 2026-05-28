package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/sausheong/wadb/internal/waclient"
)

func TestSendText_RoundTrip(t *testing.T) {
	q := seedDB(t)
	fake := waclient.NewFake()
	h := NewSendTextHandler(q, fake)
	res, err := h(context.Background(), callReq(map[string]any{
		"chat_jid": "x@s.whatsapp.net", "text": "hi",
	}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.IsError {
		t.Fatalf("isError: %+v", res)
	}
	var out struct {
		MessageID string `json:"message_id"`
	}
	json.Unmarshal([]byte(firstTextContent(t, res)), &out)
	if out.MessageID == "" {
		t.Error("MessageID empty")
	}
	if len(fake.SentText) != 1 || fake.SentText[0].Text != "hi" {
		t.Errorf("SentText = %+v", fake.SentText)
	}
}

func TestSendText_MissingArgs(t *testing.T) {
	q := seedDB(t)
	fake := waclient.NewFake()
	h := NewSendTextHandler(q, fake)
	res, _ := h(context.Background(), callReq(map[string]any{"chat_jid": "x@s.whatsapp.net"}))
	if !res.IsError {
		t.Error("expected error for missing text")
	}
}

func TestSendText_PropagatesError_Retryable(t *testing.T) {
	q := seedDB(t)
	fake := waclient.NewFake()
	fake.SendErr = errors.New("rate limited")
	h := NewSendTextHandler(q, fake)
	res, _ := h(context.Background(), callReq(map[string]any{"chat_jid": "x@s.whatsapp.net", "text": "hi"}))
	if !res.IsError {
		t.Fatal("expected error")
	}
	var got struct {
		Error     string `json:"error"`
		Retryable bool   `json:"retryable"`
	}
	json.Unmarshal([]byte(firstTextContent(t, res)), &got)
	if got.Error == "" || !got.Retryable {
		t.Errorf("envelope = %+v", got)
	}
}

func TestReact_CallsClient(t *testing.T) {
	q := seedDB(t)
	fake := waclient.NewFake()
	h := NewReactHandler(q, fake)
	res, _ := h(context.Background(), callReq(map[string]any{
		"chat_jid": "x@s.whatsapp.net", "message_id": "M1", "emoji": "👍",
	}))
	if res.IsError {
		t.Fatalf("isError: %+v", res)
	}
	if len(fake.Reactions) != 1 || fake.Reactions[0].Emoji != "👍" {
		t.Errorf("Reactions = %+v", fake.Reactions)
	}
}

func TestMarkRead_CallsClient(t *testing.T) {
	q := seedDB(t)
	fake := waclient.NewFake()
	h := NewMarkReadHandler(q, fake)
	_, _ = h(context.Background(), callReq(map[string]any{"chat_jid": "x@s.whatsapp.net"}))
	if len(fake.MarkReads) != 1 {
		t.Errorf("MarkReads = %+v", fake.MarkReads)
	}
}
