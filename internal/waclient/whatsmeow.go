package waclient

import (
	"context"
	"errors"
)

// WhatsmeowClient is the production Client. It is wired up in Task 11.
type WhatsmeowClient struct{}

func (*WhatsmeowClient) Events(context.Context) <-chan Event { return nil }
func (*WhatsmeowClient) Connected() bool                     { return false }
func (*WhatsmeowClient) DeviceJID() string                   { return "" }
func (*WhatsmeowClient) SendText(context.Context, string, string, string) (SendResult, error) {
	return SendResult{}, errors.New("not implemented")
}
func (*WhatsmeowClient) SendMedia(context.Context, string, string, []byte, string, string, string) (SendResult, error) {
	return SendResult{}, errors.New("not implemented")
}
func (*WhatsmeowClient) React(context.Context, string, string, string) error {
	return errors.New("not implemented")
}
func (*WhatsmeowClient) MarkRead(context.Context, string, string) error {
	return errors.New("not implemented")
}
func (*WhatsmeowClient) Download(context.Context, string) (DownloadResult, error) {
	return DownloadResult{}, errors.New("not implemented")
}
func (*WhatsmeowClient) Disconnect() {}

// Compile-time check that Fake and WhatsmeowClient both satisfy Client.
var _ Client = (*Fake)(nil)
var _ Client = (*WhatsmeowClient)(nil)
